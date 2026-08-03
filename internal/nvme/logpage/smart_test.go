package logpage

import (
	"encoding/binary"
	"testing"
)

func buildSmartLog() []byte {
	b := make([]byte, SmartLogSize)

	b[0] = 0x05                                // spare_below_threshold | reliability_degraded
	binary.LittleEndian.PutUint16(b[1:3], 340) // composite temperature, Kelvin
	b[3] = 100                                 // available spare
	b[4] = 10                                  // spare threshold
	b[5] = 2                                   // percentage used
	b[6] = 0x06                                // endurance group critical warning summary

	put128 := func(off int, hi, lo uint64) {
		binary.LittleEndian.PutUint64(b[off:off+8], lo)
		binary.LittleEndian.PutUint64(b[off+8:off+16], hi)
	}
	put128(32, 0, 17122402)  // data units read
	put128(48, 0, 26169783)  // data units written
	put128(64, 0, 252898188) // host read commands
	put128(80, 0, 386547283) // host write commands
	put128(96, 0, 1448)      // controller busy time, minutes
	put128(112, 0, 89)       // power cycles
	put128(128, 2, 1844)     // power on hours (non-zero Hi to exercise the upper 64 bits)
	put128(144, 0, 65)       // unsafe shutdowns
	put128(160, 0, 21)       // media and data integrity errors
	put128(176, 0, 9)        // number of error information log entries

	binary.LittleEndian.PutUint32(b[192:196], 7) // warning temp time, minutes
	binary.LittleEndian.PutUint32(b[196:200], 3) // critical temp time, minutes

	binary.LittleEndian.PutUint16(b[200:202], 304) // sensor 1
	binary.LittleEndian.PutUint16(b[202:204], 303) // sensor 2; 3..8 stay zero

	binary.LittleEndian.PutUint32(b[216:220], 11) // thermal 1 transitions
	binary.LittleEndian.PutUint32(b[220:224], 12) // thermal 2 transitions
	binary.LittleEndian.PutUint32(b[224:228], 13) // thermal 1 total time, seconds
	binary.LittleEndian.PutUint32(b[228:232], 14) // thermal 2 total time, seconds

	return b
}

func TestParseSmartFields(t *testing.T) {
	got, err := ParseSmart(buildSmartLog())
	if err != nil {
		t.Fatalf("ParseSmart: %v", err)
	}

	if got.CriticalWarning != 0x05 {
		t.Errorf("CriticalWarning = %#x, want 0x05", got.CriticalWarning)
	}
	if got.CompositeTemperatureKelvin != 340 {
		t.Errorf("CompositeTemperatureKelvin = %d, want 340", got.CompositeTemperatureKelvin)
	}
	if got.AvailableSparePercent != 100 {
		t.Errorf("AvailableSparePercent = %d, want 100", got.AvailableSparePercent)
	}
	if got.AvailableSpareThresholdPercent != 10 {
		t.Errorf("AvailableSpareThresholdPercent = %d, want 10", got.AvailableSpareThresholdPercent)
	}
	if got.PercentageUsed != 2 {
		t.Errorf("PercentageUsed = %d, want 2", got.PercentageUsed)
	}
	if got.EnduranceGroupCriticalWarning != 0x06 {
		t.Errorf("EnduranceGroupCriticalWarning = %#x, want 0x06", got.EnduranceGroupCriticalWarning)
	}
	if got.DataUnitsRead.Lo != 17122402 {
		t.Errorf("DataUnitsRead = %d, want 17122402", got.DataUnitsRead.Lo)
	}
	if got.DataUnitsWritten.Lo != 26169783 {
		t.Errorf("DataUnitsWritten = %d, want 26169783", got.DataUnitsWritten.Lo)
	}
	if got.HostReadCommands.Lo != 252898188 {
		t.Errorf("HostReadCommands = %d, want 252898188", got.HostReadCommands.Lo)
	}
	if got.HostWriteCommands.Lo != 386547283 {
		t.Errorf("HostWriteCommands = %d, want 386547283", got.HostWriteCommands.Lo)
	}
	if got.ControllerBusyTimeMinutes.Lo != 1448 {
		t.Errorf("ControllerBusyTimeMinutes = %d, want 1448", got.ControllerBusyTimeMinutes.Lo)
	}
	if got.PowerCycles.Lo != 89 {
		t.Errorf("PowerCycles = %d, want 89", got.PowerCycles.Lo)
	}
	if got.PowerOnHours.Lo != 1844 || got.PowerOnHours.Hi != 2 {
		t.Errorf("PowerOnHours = {Hi:%d Lo:%d}, want {Hi:2 Lo:1844}", got.PowerOnHours.Hi, got.PowerOnHours.Lo)
	}
	if got.UnsafeShutdowns.Lo != 65 {
		t.Errorf("UnsafeShutdowns = %d, want 65", got.UnsafeShutdowns.Lo)
	}
	if got.MediaErrors.Lo != 21 {
		t.Errorf("MediaErrors = %d, want 21", got.MediaErrors.Lo)
	}
	if got.ErrorLogEntries.Lo != 9 {
		t.Errorf("ErrorLogEntries = %d, want 9", got.ErrorLogEntries.Lo)
	}
	if got.WarningTempTimeMinutes != 7 {
		t.Errorf("WarningTempTimeMinutes = %d, want 7", got.WarningTempTimeMinutes)
	}
	if got.CriticalTempTimeMinutes != 3 {
		t.Errorf("CriticalTempTimeMinutes = %d, want 3", got.CriticalTempTimeMinutes)
	}
	if got.ThermalTransitionCount[0] != 11 || got.ThermalTransitionCount[1] != 12 {
		t.Errorf("ThermalTransitionCount = %v, want [11 12]", got.ThermalTransitionCount)
	}
	if got.ThermalTotalTimeSeconds[0] != 13 || got.ThermalTotalTimeSeconds[1] != 14 {
		t.Errorf("ThermalTotalTimeSeconds = %v, want [13 14]", got.ThermalTotalTimeSeconds)
	}
}

// Sensor count by model: 3 on Micron 7450, 2 on PM9A1, 1 on 3500, 0 on KCD8.
func TestParseSmartOmitsAbsentSensors(t *testing.T) {
	got, err := ParseSmart(buildSmartLog())
	if err != nil {
		t.Fatalf("ParseSmart: %v", err)
	}

	sensors := got.PresentSensors()
	if len(sensors) != 2 {
		t.Fatalf("got %d sensors, want 2", len(sensors))
	}
	if sensors[0].Index != 1 || sensors[0].Kelvin != 304 {
		t.Errorf("sensor[0] = %+v, want {1 304}", sensors[0])
	}
	if sensors[1].Index != 2 || sensors[1].Kelvin != 303 {
		t.Errorf("sensor[1] = %+v, want {2 303}", sensors[1])
	}
}

func TestParseSmartRejectsShortBuffer(t *testing.T) {
	for _, n := range []int{0, 1, 231, 511} {
		if _, err := ParseSmart(make([]byte, n)); err == nil {
			t.Errorf("ParseSmart(%d bytes): got no error, want one", n)
		}
	}
}

func TestParseSmartAcceptsLongerBuffer(t *testing.T) {
	// The controller may return more than asked; extra bytes are ignored.
	b := append(buildSmartLog(), make([]byte, 64)...)
	if _, err := ParseSmart(b); err != nil {
		t.Fatalf("ParseSmart on a buffer longer than 512: %v", err)
	}
}
