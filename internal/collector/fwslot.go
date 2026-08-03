package collector

import (
	"context"
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

var (
	dFirmwareSlotInfo = desc("firmware_slot_info",
		"Firmware revision held in a slot. Value is always 1. Empty slots are not reported.", "slot", "revision")
	dFirmwareActiveSlot = desc("firmware_active_slot",
		"Slot the controller is currently running firmware from.")
	dFirmwareNextSlot = desc("firmware_next_slot",
		"Slot that will be activated at the next controller reset. Not emitted when no activation is pending.")
)

func (e *Exporter) collectFirmwareSlots(ctx context.Context, ch chan<- prometheus.Metric, c nvme.Controller) error {
	if e.pageKnownUnsupported(c, "0x03") {
		e.reportPage(ch, c, "0x03", false)
		return nil
	}

	raw, err := e.src.LogPage(ctx, c.Name, logpage.IDFirmwareSlot, logpage.FirmwareSlotSize)
	if err != nil {
		if !isDeviceError(err) {
			e.markPageUnsupported(c, "0x03")
			e.reportPage(ch, c, "0x03", false)
			return nil
		}
		return err
	}
	e.reportPage(ch, c, "0x03", true)

	f, err := logpage.ParseFirmwareSlots(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", errParse, err)
	}

	for _, s := range f.PopulatedSlots() {
		ch <- prometheus.MustNewConstMetric(dFirmwareSlotInfo, prometheus.GaugeValue, 1,
			c.Name, c.Serial, strconv.Itoa(s.Slot), s.Revision)
	}

	ch <- prometheus.MustNewConstMetric(dFirmwareActiveSlot, prometheus.GaugeValue,
		float64(f.Active), c.Name, c.Serial)

	// Absence is the signal; a constant zero would not be.
	if f.Next != 0 {
		ch <- prometheus.MustNewConstMetric(dFirmwareNextSlot, prometheus.GaugeValue,
			float64(f.Next), c.Name, c.Serial)
	}

	return nil
}
