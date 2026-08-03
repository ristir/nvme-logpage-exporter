package logpage

import (
	"encoding/binary"
	"fmt"
)

// IDSmart is the identifier of the SMART / Health Information Log Page.
const IDSmart uint8 = 0x02

// SmartLogSize is the fixed size of page 0x02.
const SmartLogSize = 512

// Bits of the Critical Warning field.
const (
	WarnSpareBelowThreshold = 1 << 0
	WarnTemperature         = 1 << 1
	WarnReliabilityDegraded = 1 << 2
	WarnReadOnly            = 1 << 3
	WarnVolatileBackupFail  = 1 << 4
	WarnPersistentMemoryRO  = 1 << 5
)

// Sensor is a single temperature sensor reading.
type Sensor struct {
	Index  int // 1..8
	Kelvin uint16
}

// Smart is the parsed page 0x02. Field names carry units; the spec mixes them.
type Smart struct {
	CriticalWarning                uint8
	CompositeTemperatureKelvin     uint16
	AvailableSparePercent          uint8
	AvailableSpareThresholdPercent uint8
	PercentageUsed                 uint8

	// Parsed but not exported: no device checked has endurance groups.
	EnduranceGroupCriticalWarning uint8

	DataUnitsRead             Uint128
	DataUnitsWritten          Uint128
	HostReadCommands          Uint128
	HostWriteCommands         Uint128
	ControllerBusyTimeMinutes Uint128
	PowerCycles               Uint128
	PowerOnHours              Uint128
	UnsafeShutdowns           Uint128
	MediaErrors               Uint128
	ErrorLogEntries           Uint128

	WarningTempTimeMinutes  uint32
	CriticalTempTimeMinutes uint32

	// SensorsKelvin[i] == 0 means sensor i+1 is not present.
	SensorsKelvin [8]uint16

	ThermalTransitionCount  [2]uint32
	ThermalTotalTimeSeconds [2]uint32
}

// PresentSensors skips absent sensors, which read zero and plot as -273.
func (s *Smart) PresentSensors() []Sensor {
	var out []Sensor
	for i, k := range s.SensorsKelvin {
		if k == 0 {
			continue
		}
		out = append(out, Sensor{Index: i + 1, Kelvin: k})
	}
	return out
}

// HasWarning reports whether a given Critical Warning bit is set.
func (s *Smart) HasWarning(bit uint8) bool { return s.CriticalWarning&bit != 0 }

// ReservedWarningBits returns bits 6 and 7: undefined by the spec, but a
// drive setting one is still saying something.
func (s *Smart) ReservedWarningBits() []int {
	var out []int
	for _, bit := range []int{6, 7} {
		if s.CriticalWarning&(1<<bit) != 0 {
			out = append(out, bit)
		}
	}
	return out
}

// ParseSmart accepts a longer buffer; a shorter one is an error.
func ParseSmart(b []byte) (*Smart, error) {
	if len(b) < SmartLogSize {
		return nil, fmt.Errorf("page 0x02: got %d bytes, need at least %d", len(b), SmartLogSize)
	}

	s := &Smart{
		CriticalWarning:                b[0],
		CompositeTemperatureKelvin:     binary.LittleEndian.Uint16(b[1:3]),
		AvailableSparePercent:          b[3],
		AvailableSpareThresholdPercent: b[4],
		PercentageUsed:                 b[5],
		EnduranceGroupCriticalWarning:  b[6],

		DataUnitsRead:             readUint128(b[32:48]),
		DataUnitsWritten:          readUint128(b[48:64]),
		HostReadCommands:          readUint128(b[64:80]),
		HostWriteCommands:         readUint128(b[80:96]),
		ControllerBusyTimeMinutes: readUint128(b[96:112]),
		PowerCycles:               readUint128(b[112:128]),
		PowerOnHours:              readUint128(b[128:144]),
		UnsafeShutdowns:           readUint128(b[144:160]),
		MediaErrors:               readUint128(b[160:176]),
		ErrorLogEntries:           readUint128(b[176:192]),

		WarningTempTimeMinutes:  binary.LittleEndian.Uint32(b[192:196]),
		CriticalTempTimeMinutes: binary.LittleEndian.Uint32(b[196:200]),
	}

	for i := 0; i < 8; i++ {
		off := 200 + i*2
		s.SensorsKelvin[i] = binary.LittleEndian.Uint16(b[off : off+2])
	}

	s.ThermalTransitionCount[0] = binary.LittleEndian.Uint32(b[216:220])
	s.ThermalTransitionCount[1] = binary.LittleEndian.Uint32(b[220:224])
	s.ThermalTotalTimeSeconds[0] = binary.LittleEndian.Uint32(b[224:228])
	s.ThermalTotalTimeSeconds[1] = binary.LittleEndian.Uint32(b[228:232])

	return s, nil
}
