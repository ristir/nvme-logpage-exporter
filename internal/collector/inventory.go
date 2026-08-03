package collector

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

// The only state compared against; the rest pass through as labels.
const controllerStateLive = "live"

var (
	dDeviceInfo = prometheus.NewDesc(
		"nvme_logpage_device_info",
		"Inventory attributes of the device. Value is always 1.",
		[]string{"device", "serial", "model", "firmware", "vendor_id", "nvme_version"}, nil)

	dCapacity = desc("capacity_bytes",
		"Total NVM capacity in bytes.")
	dNamespacesMax = desc("namespaces_max",
		"Maximum number of namespaces supported by the controller (field NN). Not to be confused with the number that actually exist.")
	dNamespacesActive = desc("namespaces_active",
		"Number of namespaces that actually exist.")
	dWarnThreshold = desc("composite_temperature_warning_threshold_celsius",
		"Warning threshold for the composite temperature reported by the controller (WCTEMP). Does not apply to individual sensors.")
	dCritThreshold = desc("composite_temperature_critical_threshold_celsius",
		"Critical threshold for the composite temperature reported by the controller (CCTEMP). Does not apply to individual sensors.")

	// The kernel's list of states has changed across releases; never hardcode it.
	dControllerState = desc("controller_state",
		"Driver state of the controller from sysfs, as one series for the current state. Value is always 1. A controller that is not \"live\" refuses admin commands, which is why its log page reads time out. See nvme_logpage_controller_live for a stable series to alert on.", "state")

	// A label-borne state resets an alert's `for` timer on every change.
	dControllerLive = desc("controller_live",
		"1 if the kernel reports the controller as live, 0 for any other state. A stable series suitable for alerting; see nvme_logpage_controller_state for which state exactly.")

	dNamespaceSize = desc("namespace_size_bytes",
		"Namespace size in bytes.", "namespace")
	dNamespaceSector = desc("namespace_sector_bytes",
		"Namespace logical block size in bytes.", "namespace")
	dNamespaceMD = desc("namespace_md_info",
		"Namespace membership in an md array. Value is always 1.", "namespace", "md")
)

func (e *Exporter) collectInventory(ctx context.Context, ch chan<- prometheus.Metric, c nvme.Controller, md map[string]string) error {
	dev, serial := c.Name, c.Serial

	gauge := func(d *prometheus.Desc, v float64, extra ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, append([]string{dev, serial}, extra...)...)
	}

	// From sysfs, so still reported when the device itself is unreachable.
	if c.State != "" {
		gauge(dControllerState, 1, c.State)

		live := 0.0
		if c.State == controllerStateLive {
			live = 1
		}
		gauge(dControllerLive, live)
	}

	// Not critical: a failure here is logged, not returned.
	nss, err := e.src.Namespaces(c.Name)
	if err != nil {
		e.logger.Error("failed to enumerate namespaces", "device", dev, "err", err)
	} else {
		gauge(dNamespacesActive, float64(len(nss)))

		for _, ns := range nss {
			if ns.SizeBytes != 0 {
				gauge(dNamespaceSize, float64(ns.SizeBytes), ns.Name)
			}
			if ns.SectorBytes != 0 {
				gauge(dNamespaceSector, float64(ns.SectorBytes), ns.Name)
			}
			if arr, ok := md[ns.Name]; ok {
				gauge(dNamespaceMD, 1, ns.Name, arr)
			}
		}
	}

	raw, err := e.src.Identify(ctx, c.Name)
	if err != nil {
		if !isDeviceError(err) {
			return nil // page not supported — not a failure
		}
		return err
	}

	id, err := logpage.ParseIdentify(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", errParse, err)
	}

	// sysfs wins: one drive returned an empty model over the wire.
	model, firmware := c.Model, c.Firmware
	if model == "" {
		model = id.Model
	}
	if firmware == "" {
		firmware = id.Firmware
	}

	ch <- prometheus.MustNewConstMetric(dDeviceInfo, prometheus.GaugeValue, 1,
		dev, serial, model, firmware, vendorHex(id.VendorID), id.Version)

	if id.TotalCapacityBytes.Float64() != 0 {
		gauge(dCapacity, id.TotalCapacityBytes.Float64())
	}
	// Zero means "not reported", here and for capacity and thresholds.
	if id.ErrorLogEntries > 0 {
		e.rememberErrorLogEntries(c, id.ErrorLogEntries)
	}

	if id.MaxNamespaces != 0 {
		gauge(dNamespacesMax, float64(id.MaxNamespaces))
	}

	if id.WarnTempKelvin != 0 {
		gauge(dWarnThreshold, float64(id.WarnTempKelvin)-kelvinOffset)
	}
	if id.CritTempKelvin != 0 {
		gauge(dCritThreshold, float64(id.CritTempKelvin)-kelvinOffset)
	}

	return nil
}

func vendorHex(v uint16) string {
	const digits = "0123456789abcdef"
	return "0x" + string([]byte{
		digits[(v>>12)&0xF], digits[(v>>8)&0xF], digits[(v>>4)&0xF], digits[v&0xF],
	})
}
