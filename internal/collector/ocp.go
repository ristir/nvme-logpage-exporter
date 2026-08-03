package collector

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

// The OCP Endurance Estimate unit: 10^9 bytes, not 2^30.
const enduranceEstimateUnitBytes = 1e9

var (
	dMediaWritten = desc("media_written_bytes_total",
		"Bytes written to the NAND, from the OCP extended health log. Divided by nvme_logpage_written_bytes_total this gives write amplification.")
	dMediaRead = desc("media_read_bytes_total",
		"Bytes read from the NAND, from the OCP extended health log.")
	dBadNANDBlocks = desc("bad_nand_blocks_total",
		"Number of NAND blocks retired, by area.", "area")
	dBadNANDNormalized = desc("bad_nand_blocks_normalized",
		"Vendor-normalized health value for retired NAND blocks, by area. 100 is nominal and the value decreases with wear; the scale is vendor-defined.", "area")
	dXORRecovery = desc("xor_recovery_total",
		"Number of times the controller recovered data using XOR parity.")
	dUncorrectableReads = desc("uncorrectable_read_errors_total",
		"Number of read errors the controller could not correct.")
	dSoftECCErrors = desc("soft_ecc_errors_total",
		"Number of correctable ECC errors.")
	dE2EDetected = desc("e2e_errors_detected_total",
		"Number of end-to-end data protection errors detected.")
	dE2ECorrected = desc("e2e_errors_corrected_total",
		"Number of end-to-end data protection errors corrected.")
	dSystemAreaUsed = desc("system_area_used_ratio",
		"Consumed endurance of the controller's system data area as a fraction, 0..1.")
	dRefreshCount = desc("refresh_count_total",
		"Number of block refresh operations performed by the controller.")
	dEraseCyclesMax = desc("user_erase_cycles_max",
		"Highest program/erase cycle count across user-data blocks.")
	dEraseCyclesMin = desc("user_erase_cycles_min",
		"Lowest program/erase cycle count across user-data blocks.")
	dThrottleEvents = desc("thermal_throttle_events_total",
		"Number of thermal throttling activations, from the OCP log (page 0xC0). The field is one byte wide and saturates at 255 instead of wrapping. See nvme_logpage_thermal_transitions_total for the health log's own count of the same kind of event (page 0x02).")
	dThrottleRatio = desc("thermal_throttle_ratio",
		"Throttling currently applied, as a fraction 0..1.")
	dPCIeCorrectable = desc("pcie_correctable_errors_total",
		"Number of correctable errors reported by the PCIe link.")
	dIncompleteShutdowns = desc("incomplete_shutdowns_total",
		"Number of shutdowns the controller did not complete cleanly, from the OCP log (page 0xC0). See nvme_logpage_unsafe_shutdowns_total for the health log's own count of unsafe shutdowns (page 0x02).")
	dFreeBlocks = desc("free_blocks_ratio",
		"Free NAND blocks as a fraction of the total, 0..1.")
	dCapacitorHealth = desc("capacitor_health",
		"Power-loss-protection capacitor health, on a vendor-defined scale. Not a percentage: values above 100 are reported by real controllers.")
	dUnalignedIO = desc("unaligned_io_total",
		"Number of host I/O operations not aligned to the controller's internal granularity.")
	dSecurityVersion = desc("security_version",
		"Security version number reported by the controller.")
	dNamespaceUsed = desc("namespace_used_bytes",
		"Bytes in use on namespace 1, from the OCP NUSE field, which is defined for that namespace alone. On a controller with several namespaces the others are not covered, so this is not whole-device utilization. Carries a namespace label so it divides directly against nvme_logpage_namespace_size_bytes.", "namespace")
	dPLPStart = desc("plp_starts_total",
		"Number of times power-loss protection engaged.")
	dEnduranceEstimate = desc("endurance_estimate_bytes",
		"Manufacturer's estimate of total bytes writable over the drive's lifetime at a write amplification of 1. The log field is in units of 10^9 bytes.")
	dOCPInfo = desc("ocp_info",
		"Presence of an OCP extended health log, with its version as a label. Value is always 1.", "version")
)

func (e *Exporter) namespaceOne(ctrl string) (string, bool) {
	nss, err := e.src.Namespaces(ctrl)
	if err != nil {
		return "", false
	}
	want := ctrl + "n1"
	for _, ns := range nss {
		if ns.Name == want {
			return ns.Name, true
		}
	}
	return "", false
}

func (e *Exporter) collectOCP(ctx context.Context, ch chan<- prometheus.Metric, c nvme.Controller) error {
	if e.pageKnownUnsupported(c, "0xc0") {
		e.reportPage(ch, c, "0xc0", false)
		return nil
	}

	raw, err := e.src.LogPage(ctx, c.Name, logpage.IDOCPSmart, logpage.OCPSmartSize)
	if err != nil {
		if !isDeviceError(err) {
			e.markPageUnsupported(c, "0xc0")
			e.reportPage(ch, c, "0xc0", false)
			return nil
		}
		// A device failure says nothing about page support.
		return err
	}

	p, err := logpage.ParseOCPSmart(raw)
	if err != nil {
		if errors.Is(err, logpage.ErrNotOCP) {
			// A foreign GUID is a permanent answer, same as a refusal.
			e.markPageUnsupported(c, "0xc0")
			e.reportPage(ch, c, "0xc0", false)
			return nil
		}
		// The page exists, so the gauge says so even if this scrape cannot decode it.
		e.reportPage(ch, c, "0xc0", true)
		return fmt.Errorf("%w: %v", errParse, err)
	}
	e.reportPage(ch, c, "0xc0", true)

	ch <- prometheus.MustNewConstMetric(dOCPInfo, prometheus.GaugeValue, 1,
		c.Name, c.Serial, strconv.FormatUint(uint64(p.Version), 10))

	wide := []struct {
		d *prometheus.Desc
		v logpage.OptU128
		t prometheus.ValueType
		s float64
	}{
		{dMediaWritten, p.PhysicalMediaWrittenBytes, prometheus.CounterValue, 1},
		{dMediaRead, p.PhysicalMediaReadBytes, prometheus.CounterValue, 1},
		{dPLPStart, p.PLPStartCount, prometheus.CounterValue, 1},
		{dEnduranceEstimate, p.EnduranceEstimateGB, prometheus.GaugeValue, enduranceEstimateUnitBytes},
	}
	for _, m := range wide {
		if !m.v.Present {
			continue
		}
		ch <- prometheus.MustNewConstMetric(m.d, m.t, m.v.Value.Float64()*m.s, c.Name, c.Serial)
	}

	narrow := []struct {
		d      *prometheus.Desc
		v      logpage.OptU64
		t      prometheus.ValueType
		s      float64
		labels []string
	}{
		{dBadNANDBlocks, p.BadUserNANDBlocksRaw, prometheus.CounterValue, 1, []string{"user"}},
		{dBadNANDBlocks, p.BadSystemNANDBlocksRaw, prometheus.CounterValue, 1, []string{"system"}},
		{dBadNANDNormalized, p.BadUserNANDBlocksNormalized, prometheus.GaugeValue, 1, []string{"user"}},
		{dBadNANDNormalized, p.BadSystemNANDBlocksNormalized, prometheus.GaugeValue, 1, []string{"system"}},
		{dXORRecovery, p.XORRecoveryCount, prometheus.CounterValue, 1, nil},
		{dUncorrectableReads, p.UncorrectableReadErrors, prometheus.CounterValue, 1, nil},
		{dSoftECCErrors, p.SoftECCErrors, prometheus.CounterValue, 1, nil},
		{dE2EDetected, p.E2EDetectedErrors, prometheus.CounterValue, 1, nil},
		{dE2ECorrected, p.E2ECorrectedErrors, prometheus.CounterValue, 1, nil},
		{dSystemAreaUsed, p.SystemDataPercentUsed, prometheus.GaugeValue, 0.01, nil},
		{dRefreshCount, p.RefreshCount, prometheus.CounterValue, 1, nil},
		{dEraseCyclesMax, p.MaxUserDataEraseCount, prometheus.GaugeValue, 1, nil},
		{dEraseCyclesMin, p.MinUserDataEraseCount, prometheus.GaugeValue, 1, nil},
		{dThrottleEvents, p.ThermalThrottleEvents, prometheus.CounterValue, 1, nil},
		{dThrottleRatio, p.ThermalThrottleStatusPercent, prometheus.GaugeValue, 0.01, nil},
		{dPCIeCorrectable, p.PCIeCorrectableErrors, prometheus.CounterValue, 1, nil},
		{dIncompleteShutdowns, p.IncompleteShutdowns, prometheus.CounterValue, 1, nil},
		{dFreeBlocks, p.PercentFreeBlocks, prometheus.GaugeValue, 0.01, nil},
		{dCapacitorHealth, p.CapacitorHealth, prometheus.GaugeValue, 1, nil},
		{dUnalignedIO, p.UnalignedIO, prometheus.CounterValue, 1, nil},
		{dSecurityVersion, p.SecurityVersion, prometheus.GaugeValue, 1, nil},
	}

	// NUSE is namespace 1; sysfs directory order is not namespace order.
	// Skipped when absent: the Desc has a fixed label count.
	if ns, ok := e.namespaceOne(c.Name); ok {
		narrow = append(narrow, struct {
			d      *prometheus.Desc
			v      logpage.OptU64
			t      prometheus.ValueType
			s      float64
			labels []string
		}{dNamespaceUsed, p.NamespaceUtilizationBytes, prometheus.GaugeValue, 1, []string{ns}})
	}

	for _, m := range narrow {
		if !m.v.Present {
			continue
		}
		labels := append([]string{c.Name, c.Serial}, m.labels...)
		ch <- prometheus.MustNewConstMetric(m.d, m.t, float64(m.v.Value)*m.s, labels...)
	}

	return nil
}
