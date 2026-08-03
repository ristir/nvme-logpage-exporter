// Package collector turns parsed NVMe structures into Prometheus metrics.
package collector

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
)

var (
	dDevicesDiscovered = prometheus.NewDesc(
		"nvme_logpage_devices_discovered",
		"Number of NVMe controllers found during enumeration.", nil, nil)

	dScrapeSuccess = desc("scrape_success",
		"1 if the device was polled successfully in full; 0 on any failure.")
	dScrapeDuration = desc("scrape_duration_seconds",
		"Duration of polling a single device, in seconds.")
	dDeviceErrors = desc("errors_total",
		"Number of device-polling failures, broken down by reason. Counts failures to poll a device, not exporter errors in general.", "reason")
	dLogPageAvailable = desc("supported",
		"1 if the controller serves this log page; 0 if it does not support it.", "page")
)

// Exporter polls every controller each scrape; nothing is cached.
type Exporter struct {
	src    nvme.Source
	logger *slog.Logger

	ctx context.Context

	deviceTimeout time.Duration

	// Filled by Handler; re-enumerating here would be a TOCTOU. nil in tests.
	ctrls []nvme.Controller

	// Pointer: Handler shallow-copies Exporter, and a mutex must not be copied.
	state *scrapeState
}

type errKey struct {
	device string
	serial string
	reason string
}

// Serial is part of the key so a drive swap resets the state.
type pageKey struct {
	device string
	serial string
	page   string
}

// deviceKey identifies a controller; serial included so a swap resets state.
type deviceKey struct {
	device string
	serial string
}

type scrapeState struct {
	mu sync.Mutex

	errCounts map[errKey]float64

	// One bad drive would otherwise log 1440 identical lines a day.
	logged map[errKey]struct{}

	// Asked once each; see pageKnownUnsupported for why re-asking is not free.
	pageUnsupported map[pageKey]struct{}

	// ELPE never changes for a controller, and Identify is read before the
	// error log anyway, so the size travels through here rather than costing
	// a second Identify per scrape.
	errorLogEntries map[deviceKey]int
}

// New builds an Exporter.
func New(src nvme.Source, logger *slog.Logger) *Exporter {
	return &Exporter{
		src:           src,
		logger:        logger,
		ctx:           context.Background(),
		deviceTimeout: 5 * time.Second,
		state: &scrapeState{
			errCounts:       make(map[errKey]float64),
			logged:          make(map[errKey]struct{}),
			pageUnsupported: map[pageKey]struct{}{},
			errorLogEntries: map[deviceKey]int{},
		},
	}
}

// Describe emits nothing: the metric set varies per device.
func (e *Exporter) Describe(chan<- *prometheus.Desc) {}

// Collect polls every controller and emits its metrics.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	ctrls := e.ctrls
	if ctrls == nil {
		var err error
		ctrls, err = e.src.Controllers()
		if err != nil {
			e.logger.Error("failed to enumerate controllers", "err", err)
			return
		}
	}

	ch <- prometheus.MustNewConstMetric(dDevicesDiscovered, prometheus.GaugeValue, float64(len(ctrls)))

	// Host-wide, so read once per scrape rather than once per controller.
	md, err := e.src.MDMembership()
	if err != nil {
		e.logger.Error("failed to read md array membership", "err", err)
		md = map[string]string{}
	}

	workers := runtime.NumCPU()
	if workers > len(ctrls) {
		workers = len(ctrls)
	}
	if workers < 1 {
		workers = 1
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, c := range ctrls {
		wg.Add(1)
		go func(c nvme.Controller) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(e.ctx, e.deviceTimeout)
			defer cancel()

			e.collectController(ctx, ch, c, md)
		}(c)
	}
	wg.Wait()

	e.emitErrorCounters(ch)
}

func (e *Exporter) collectController(ctx context.Context, ch chan<- prometheus.Metric, c nvme.Controller, md map[string]string) {
	start := time.Now()
	ok := true

	if err := e.collectInventory(ctx, ch, c, md); err != nil {
		ok = false
		e.recordError(c, err)
	}
	if err := e.collectSmart(ctx, ch, c); err != nil {
		ok = false
		e.recordError(c, err)
	}
	if err := e.collectOCP(ctx, ch, c); err != nil {
		ok = false
		e.recordError(c, err)
	}
	if err := e.collectFirmwareSlots(ctx, ch, c); err != nil {
		ok = false
		e.recordError(c, err)
	}
	if err := e.collectErrorInfo(ctx, ch, c); err != nil {
		ok = false
		e.recordError(c, err)
	}

	if ok {
		// A recurrence after recovery must be logged again.
		e.clearLogged(c)
	}

	success := 0.0
	if ok {
		success = 1
	}
	ch <- prometheus.MustNewConstMetric(dScrapeSuccess, prometheus.GaugeValue, success, c.Name, c.Serial)
	ch <- prometheus.MustNewConstMetric(dScrapeDuration, prometheus.GaugeValue, time.Since(start).Seconds(), c.Name, c.Serial)
}

// Logged once per reason; the counter still rises every time.
func (e *Exporter) recordError(c nvme.Controller, err error) {
	k := errKey{device: c.Name, serial: c.Serial, reason: classifyError(err)}

	e.state.mu.Lock()
	e.state.errCounts[k]++
	_, seen := e.state.logged[k]
	if !seen {
		e.state.logged[k] = struct{}{}
	}
	e.state.mu.Unlock()

	if !seen {
		e.logger.Error("device polling failed",
			"device", c.Name, "serial", c.Serial, "reason", k.reason, "err", err)
	}
}

// errCounts is untouched: a counter must never go down.
func (e *Exporter) clearLogged(c nvme.Controller) {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()

	for k := range e.state.logged {
		if k.device == c.Name && k.serial == c.Serial {
			delete(e.state.logged, k)
		}
	}
}

func (e *Exporter) emitErrorCounters(ch chan<- prometheus.Metric) {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()

	for k, v := range e.state.errCounts {
		ch <- prometheus.MustNewConstMetric(dDeviceErrors, prometheus.CounterValue, v, k.device, k.serial, k.reason)
	}
}

// Each refused read appends an entry to the drive's own error log.
func (e *Exporter) pageKnownUnsupported(c nvme.Controller, page string) bool {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()

	_, ok := e.state.pageUnsupported[pageKey{device: c.Name, serial: c.Serial, page: page}]
	return ok
}

func (e *Exporter) markPageUnsupported(c nvme.Controller, page string) {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()

	e.state.pageUnsupported[pageKey{device: c.Name, serial: c.Serial, page: page}] = struct{}{}
}

func (e *Exporter) rememberErrorLogEntries(c nvme.Controller, n int) {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()

	e.state.errorLogEntries[deviceKey{device: c.Name, serial: c.Serial}] = n
}

func (e *Exporter) knownErrorLogEntries(c nvme.Controller) int {
	e.state.mu.Lock()
	defer e.state.mu.Unlock()

	return e.state.errorLogEntries[deviceKey{device: c.Name, serial: c.Serial}]
}

// Absent is routine, not a failure.
func (e *Exporter) reportPage(ch chan<- prometheus.Metric, c nvme.Controller, page string, available bool) {
	v := 0.0
	if available {
		v = 1
	}
	ch <- prometheus.MustNewConstMetric(dLogPageAvailable, prometheus.GaugeValue, v, c.Name, c.Serial, page)
}

var _ prometheus.Collector = (*Exporter)(nil)
