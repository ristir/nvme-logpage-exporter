package logpage

import (
	"encoding/binary"
	"testing"
)

func FuzzParseOCPSmart(f *testing.F) {
	valid := make([]byte, OCPSmartSize)
	copy(valid[496:512], ocpGUID[:])
	f.Add(valid)
	f.Add(make([]byte, OCPSmartSize))
	f.Add(make([]byte, OCPSmartSize-1)) // truncated but non-empty
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := ParseOCPSmart(b)
		if err != nil {
			return
		}

		if got, want := p.Version, binary.LittleEndian.Uint16(b[494:496]); got != want {
			t.Fatalf("Version = %#x, want %#x", got, want)
		}

		checkU16Sentinel(t, "CapacitorHealth", b, 128, p.CapacitorHealth.Present)
		checkU64Sentinel(t, "SecurityVersion", b, 144, p.SecurityVersion.Present)
	})
}

func checkU16Sentinel(t *testing.T, name string, b []byte, off int, present bool) {
	t.Helper()
	raw := binary.LittleEndian.Uint16(b[off : off+2])
	if present && raw == 0xFFFF {
		t.Fatalf("%s: Present = true but the buffer reads all ones", name)
	}
	if !present && raw != 0xFFFF {
		t.Fatalf("%s: Present = false but the buffer reads %#x, not all ones", name, raw)
	}
}

func checkU64Sentinel(t *testing.T, name string, b []byte, off int, present bool) {
	t.Helper()
	raw := binary.LittleEndian.Uint64(b[off : off+8])
	if present && raw == ^uint64(0) {
		t.Fatalf("%s: Present = true but the buffer reads all ones", name)
	}
	if !present && raw != ^uint64(0) {
		t.Fatalf("%s: Present = false but the buffer reads %#x, not all ones", name, raw)
	}
}

func FuzzParseFirmwareSlots(f *testing.F) {
	f.Add(fwPage(0x02, "VDV1DP23", "VDV1DP25"))
	f.Add(make([]byte, FirmwareSlotSize))
	f.Add(make([]byte, FirmwareSlotSize-1)) // truncated but non-empty
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, b []byte) {
		s, err := ParseFirmwareSlots(b)
		if err != nil {
			return
		}

		var want int
		for _, r := range s.Revisions {
			if r != "" {
				want++
			}
		}

		got := s.PopulatedSlots()
		if len(got) != want {
			t.Fatalf("PopulatedSlots() returned %d entries, want %d non-empty revisions", len(got), want)
		}
		for _, slot := range got {
			if slot.Revision == "" {
				t.Fatalf("PopulatedSlots() returned slot %d with an empty revision", slot.Slot)
			}
		}
	})
}

func FuzzParseErrorInfo(f *testing.F) {
	f.Add(realWorldEntry())
	f.Add(make([]byte, 512))
	f.Add(make([]byte, ErrorInfoEntrySize))
	f.Add(make([]byte, ErrorInfoEntrySize-1)) // truncated but non-empty
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, b []byte) {
		entries, err := ParseErrorInfo(b)
		if err != nil {
			return
		}
		if len(entries) > len(b)/ErrorInfoEntrySize {
			t.Fatalf("got %d entries from %d bytes", len(entries), len(b))
		}
	})
}
