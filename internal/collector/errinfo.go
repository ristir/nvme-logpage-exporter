package collector

import (
	"context"
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

// Used until Identify reports ELPE, and for controllers whose Identify fails.
const errorInfoFallbackSize = 8 * logpage.ErrorInfoEntrySize

var dErrorLogRetained = desc("error_log_retained_entries",
	"Number of entries retained in the Error Information log, grouped by status. The whole log is read: its length comes from ELPE in Identify Controller, which is 64 to 256 entries on the hardware surveyed. Diagnostic only: the log survives resets and says nothing about when the errors happened. See nvme_logpage_error_log_entries_total for the running total of entries ever added. status_code_type is decimal, as nvme-cli and the specification write it; status_code is hexadecimal, as this exporter's other page-label fields are.",
	"status_code_type", "status_code")

// The whole log, not a fixed slice of it: nine drives here fill every one of
// the eight entries a 512-byte read returns, so a fixed read silently truncates.
func (e *Exporter) errorInfoSize(c nvme.Controller) int {
	if n := e.knownErrorLogEntries(c); n > 0 {
		return n * logpage.ErrorInfoEntrySize
	}
	return errorInfoFallbackSize
}

// Aggregated by status: a series per entry would be up to 256 per device.
func (e *Exporter) collectErrorInfo(ctx context.Context, ch chan<- prometheus.Metric, c nvme.Controller) error {
	if e.pageKnownUnsupported(c, "0x01") {
		e.reportPage(ch, c, "0x01", false)
		return nil
	}

	raw, err := e.src.LogPage(ctx, c.Name, logpage.IDErrorInfo, e.errorInfoSize(c))
	if err != nil {
		if !isDeviceError(err) {
			e.markPageUnsupported(c, "0x01")
			e.reportPage(ch, c, "0x01", false)
			return nil
		}
		return err
	}
	e.reportPage(ch, c, "0x01", true)

	entries, err := logpage.ParseErrorInfo(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", errParse, err)
	}

	type key struct {
		sct uint8
		sc  uint8
	}
	counts := map[key]float64{}
	for _, en := range entries {
		counts[key{en.StatusCodeType, en.StatusCode}]++
	}

	for k, n := range counts {
		ch <- prometheus.MustNewConstMetric(dErrorLogRetained, prometheus.GaugeValue, n,
			c.Name, c.Serial,
			strconv.FormatUint(uint64(k.sct), 10),
			fmt.Sprintf("0x%02x", k.sc),
		)
	}

	return nil
}
