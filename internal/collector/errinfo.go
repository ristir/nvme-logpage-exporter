package collector

import (
	"context"
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

// Eight entries. Live drives with 88 and 314 lifetime errors populated 4 and 1.
const errorInfoSize = 512

var dErrorLogRetained = desc("error_log_retained_entries",
	"Number of entries among the first 8 the controller returns from the Error Information log, grouped by status. The log itself holds ELPE+1 entries, 64 to 256 on the hardware surveyed, so this is a sample of the most recent errors rather than the whole log. Diagnostic only: the log survives resets and says nothing about when the errors happened. See nvme_logpage_error_log_entries_total for the running total of entries ever added. status_code_type is decimal, as nvme-cli and the specification write it; status_code is hexadecimal, as this exporter's other page-label fields are.",
	"status_code_type", "status_code")

// Aggregated by status: a series per entry would be up to 256 per device.
func (e *Exporter) collectErrorInfo(ctx context.Context, ch chan<- prometheus.Metric, c nvme.Controller) error {
	if e.pageKnownUnsupported(c, "0x01") {
		e.reportPage(ch, c, "0x01", false)
		return nil
	}

	raw, err := e.src.LogPage(ctx, c.Name, logpage.IDErrorInfo, errorInfoSize)
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
