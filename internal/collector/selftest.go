package collector

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

var (
	dSelfTestRunning = desc("self_test_running",
		"1 while a device self-test is in progress; 0 when the drive is idle.")
	dSelfTestCompletion = desc("self_test_completion_ratio",
		"Progress of the running self-test as a fraction, 0..1. Only emitted while one is running.")
	dSelfTestResults = desc("self_test_results",
		"Entries retained in the device self-test log, grouped by outcome and test type. The log holds twenty entries; a drive that has never been asked to self-test reports none.", "result", "test")
	dSelfTestLastResult = desc("self_test_last_result",
		"Outcome code of the most recent self-test entry: 0 completed without error, 1-4 and 8-9 aborted, 5 fatal error, 6-7 completed with failed segments.")
	dSelfTestLastPowerOn = desc("self_test_last_power_on_seconds",
		"Drive power-on time when the most recent self-test ran. Subtract from nvme_logpage_power_on_seconds_total for the age of the result.")
)

var selfTestResultNames = map[uint8]string{
	0: "passed",
	1: "aborted_by_command",
	2: "aborted_by_reset",
	3: "aborted_namespace_removed",
	4: "aborted_by_format",
	5: "fatal_error",
	6: "failed_unknown_segment",
	7: "failed_segment",
	8: "aborted_unknown",
	9: "aborted_by_sanitize",
}

var selfTestCodeNames = map[uint8]string{
	1:    "short",
	2:    "extended",
	0x0E: "vendor",
}

func selfTestResultName(v uint8) string {
	if s, ok := selfTestResultNames[v]; ok {
		return s
	}
	return "reserved"
}

func selfTestCodeName(v uint8) string {
	if s, ok := selfTestCodeNames[v]; ok {
		return s
	}
	return "reserved"
}

const hoursToSeconds = 3600

func (e *Exporter) collectSelfTest(ctx context.Context, ch chan<- prometheus.Metric, c nvme.Controller) error {
	if e.pageKnownUnsupported(c, "0x06") {
		e.reportPage(ch, c, "0x06", false)
		return nil
	}

	raw, err := e.src.LogPage(ctx, c.Name, logpage.IDSelfTest, logpage.SelfTestSize)
	if err != nil {
		if !isDeviceError(err) {
			e.markPageUnsupported(c, "0x06")
			e.reportPage(ch, c, "0x06", false)
			return nil
		}
		return err
	}
	e.reportPage(ch, c, "0x06", true)

	s, err := logpage.ParseSelfTest(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", errParse, err)
	}

	var running float64
	if s.InProgress != 0 {
		running = 1
	}
	ch <- prometheus.MustNewConstMetric(dSelfTestRunning, prometheus.GaugeValue,
		running, c.Name, c.Serial)

	// Absence is the signal; a constant zero would not be.
	if running == 1 {
		ch <- prometheus.MustNewConstMetric(dSelfTestCompletion, prometheus.GaugeValue,
			float64(s.Completion)/100, c.Name, c.Serial)
	}

	counts := map[[2]string]float64{}
	for _, r := range s.Results {
		counts[[2]string{selfTestResultName(r.Result), selfTestCodeName(r.Code)}]++
	}
	for k, v := range counts {
		ch <- prometheus.MustNewConstMetric(dSelfTestResults, prometheus.GaugeValue, v,
			c.Name, c.Serial, k[0], k[1])
	}

	// Entry zero is the newest run.
	if len(s.Results) > 0 {
		last := s.Results[0]
		ch <- prometheus.MustNewConstMetric(dSelfTestLastResult, prometheus.GaugeValue,
			float64(last.Result), c.Name, c.Serial)
		ch <- prometheus.MustNewConstMetric(dSelfTestLastPowerOn, prometheus.GaugeValue,
			float64(last.PowerOnHours)*hoursToSeconds, c.Name, c.Serial)
	}

	return nil
}
