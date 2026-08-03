package collector

import (
	"context"
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

// 1000 blocks of 512 bytes, per spec — never derive from hw_sector_size.
const dataUnitBytes = 512000

// The exact offset. Not 273.
const kelvinOffset = 273.15

var deviceLabels = []string{"device", "serial"}

func desc(name, help string, extra ...string) *prometheus.Desc {
	return prometheus.NewDesc("nvme_logpage_"+name, help, append(append([]string{}, deviceLabels...), extra...), nil)
}

var (
	dCriticalWarningFlag = desc("critical_warning_flag",
		"A bit of the Critical Warning field from the health log: 1 if set.", "flag")
	dTemperature = desc("temperature_celsius",
		"Temperature in degrees Celsius of an individual numbered sensor (1..8). See nvme_logpage_composite_temperature_celsius for the composite reading.", "sensor")
	dCompositeTemperature = desc("composite_temperature_celsius",
		"Composite temperature in degrees Celsius, as reported by the controller.")
	dAvailableSpare = desc("available_spare_ratio",
		"Available spare capacity as a fraction of the original, 0..1.")
	dAvailableSpareThreshold = desc("available_spare_threshold_ratio",
		"Spare capacity threshold below which the controller raises a warning, 0..1.")
	dEnduranceUsed = desc("endurance_used_ratio",
		"Consumed write endurance as a fraction, 0..1. May exceed 1: the specification allows values above 100 percent.")
	dReadBytes = desc("read_bytes_total",
		"Bytes read by the host. Converted from data units, multiplier 512000.")
	dWrittenBytes = desc("written_bytes_total",
		"Bytes written by the host. Converted from data units, multiplier 512000.")
	dHostReads = desc("host_read_commands_total",
		"Number of read commands received from the host.")
	dHostWrites = desc("host_write_commands_total",
		"Number of write commands received from the host.")
	dControllerBusy = desc("controller_busy_seconds_total",
		"Time the controller spent processing commands, in seconds. The specification field is in minutes.")
	dPowerCycles = desc("power_cycles_total",
		"Number of power cycles.")
	dPowerOn = desc("power_on_seconds_total",
		"Power-on time in seconds. The specification field is in hours.")
	dUnsafeShutdowns = desc("unsafe_shutdowns_total",
		"Number of unsafe shutdowns, from the health log (page 0x02). See nvme_logpage_incomplete_shutdowns_total for the OCP log's own count of shutdowns not completed cleanly (page 0xC0).")
	dMediaErrors = desc("media_errors_total",
		"Number of unrecovered data integrity errors.")
	dErrorLogEntries = desc("error_log_entries_total",
		"Number of entries added to the Error Information Log over the drive's lifetime. See nvme_logpage_error_log_retained_entries for a breakdown by status of the most recent ones.")
	dWarningTempTime = desc("warning_temperature_seconds_total",
		"Time in seconds spent above the warning temperature threshold. The specification field is in minutes.")
	dCriticalTempTime = desc("critical_temperature_seconds_total",
		"Time in seconds spent above the critical temperature threshold. The specification field is in minutes.")
	dThermalTransitions = desc("thermal_transitions_total",
		"Number of thermal throttling activations, from the health log (page 0x02), by level. See nvme_logpage_thermal_throttle_events_total for the OCP log's own count of the same kind of event (page 0xC0).", "level")
	dThermalSeconds = desc("thermal_seconds_total",
		"Total time spent in thermal throttling, in seconds. The specification field is already in seconds.", "level")
)

// Zeros included, or "no series" and "all clear" become indistinguishable.
var knownWarningFlags = []struct {
	bit  uint8
	name string
}{
	{logpage.WarnSpareBelowThreshold, "spare_below_threshold"},
	{logpage.WarnTemperature, "temperature"},
	{logpage.WarnReliabilityDegraded, "reliability_degraded"},
	{logpage.WarnReadOnly, "read_only"},
	{logpage.WarnVolatileBackupFail, "volatile_backup_failed"},
	{logpage.WarnPersistentMemoryRO, "persistent_memory_ro"},
}

func (e *Exporter) collectSmart(ctx context.Context, ch chan<- prometheus.Metric, c nvme.Controller) error {
	if e.pageKnownUnsupported(c, "0x02") {
		e.reportPage(ch, c, "0x02", false)
		return nil
	}

	raw, err := e.src.LogPage(ctx, c.Name, logpage.IDSmart, logpage.SmartLogSize)
	if err != nil {
		if !isDeviceError(err) {
			// Routine, not a failure.
			e.markPageUnsupported(c, "0x02")
			e.reportPage(ch, c, "0x02", false)
			return nil
		}
		// A transient fault says nothing about page support, so no gauge here.
		return err
	}
	e.reportPage(ch, c, "0x02", true)

	s, err := logpage.ParseSmart(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", errParse, err)
	}

	dev, serial := c.Name, c.Serial

	gauge := func(d *prometheus.Desc, v float64, extra ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, append([]string{dev, serial}, extra...)...)
	}
	counter := func(d *prometheus.Desc, v float64, extra ...string) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v, append([]string{dev, serial}, extra...)...)
	}

	for _, f := range knownWarningFlags {
		v := 0.0
		if s.HasWarning(f.bit) {
			v = 1
		}
		gauge(dCriticalWarningFlag, v, f.name)
	}
	// Only when set: reserved_* == 1 means the drive reports something new.
	for _, bit := range s.ReservedWarningBits() {
		gauge(dCriticalWarningFlag, 1, "reserved_"+strconv.Itoa(bit))
	}

	if s.CompositeTemperatureKelvin != 0 {
		gauge(dCompositeTemperature, float64(s.CompositeTemperatureKelvin)-kelvinOffset)
	}
	for _, sensor := range s.PresentSensors() {
		gauge(dTemperature, float64(sensor.Kelvin)-kelvinOffset, strconv.Itoa(sensor.Index))
	}

	gauge(dAvailableSpare, float64(s.AvailableSparePercent)/100)
	gauge(dAvailableSpareThreshold, float64(s.AvailableSpareThresholdPercent)/100)
	gauge(dEnduranceUsed, float64(s.PercentageUsed)/100)

	counter(dReadBytes, s.DataUnitsRead.Float64()*dataUnitBytes)
	counter(dWrittenBytes, s.DataUnitsWritten.Float64()*dataUnitBytes)
	counter(dHostReads, s.HostReadCommands.Float64())
	counter(dHostWrites, s.HostWriteCommands.Float64())
	counter(dControllerBusy, s.ControllerBusyTimeMinutes.Float64()*60)
	counter(dPowerCycles, s.PowerCycles.Float64())
	counter(dPowerOn, s.PowerOnHours.Float64()*3600)
	counter(dUnsafeShutdowns, s.UnsafeShutdowns.Float64())
	counter(dMediaErrors, s.MediaErrors.Float64())
	counter(dErrorLogEntries, s.ErrorLogEntries.Float64())
	counter(dWarningTempTime, float64(s.WarningTempTimeMinutes)*60)
	counter(dCriticalTempTime, float64(s.CriticalTempTimeMinutes)*60)

	// Only a small minority of drives report these; zeros are not emitted.
	for i := 0; i < 2; i++ {
		level := strconv.Itoa(i + 1)
		if s.ThermalTransitionCount[i] != 0 {
			counter(dThermalTransitions, float64(s.ThermalTransitionCount[i]), level)
		}
		if s.ThermalTotalTimeSeconds[i] != 0 {
			counter(dThermalSeconds, float64(s.ThermalTotalTimeSeconds[i]), level)
		}
	}

	return nil
}
