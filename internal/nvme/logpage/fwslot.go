package logpage

import "fmt"

// IDFirmwareSlot is the Firmware Slot Information log page.
const IDFirmwareSlot uint8 = 0x03

// FirmwareSlotSize is the fixed size of page 0x03.
const FirmwareSlotSize = 512

const firmwareSlotCount = 7

// FirmwareSlot is one populated slot.
type FirmwareSlot struct {
	Slot     int // 1..7
	Revision string
}

// FirmwareSlots is the parsed page 0x03; Next is 0 when nothing is pending.
type FirmwareSlots struct {
	Active int
	Next   int

	// Indexed from zero for slot 1; empty means the slot holds no firmware.
	Revisions [firmwareSlotCount]string
}

// PopulatedSlots skips empty slots, which would carry an empty label.
func (f *FirmwareSlots) PopulatedSlots() []FirmwareSlot {
	var out []FirmwareSlot
	for i, r := range f.Revisions {
		if r == "" {
			continue
		}
		out = append(out, FirmwareSlot{Slot: i + 1, Revision: r})
	}
	return out
}

// ParseFirmwareSlots parses page 0x03.
func ParseFirmwareSlots(b []byte) (*FirmwareSlots, error) {
	if len(b) < FirmwareSlotSize {
		return nil, fmt.Errorf("page 0x03: got %d bytes, need at least %d", len(b), FirmwareSlotSize)
	}

	f := &FirmwareSlots{
		Active: int(b[0] & 0x07),
		Next:   int(b[0] >> 4 & 0x07),
	}
	for i := range f.Revisions {
		f.Revisions[i] = asciiField(b[8+i*8 : 16+i*8])
	}
	return f, nil
}
