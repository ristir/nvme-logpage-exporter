package logpage

import "testing"

// Built from the Dell Express Flash P4510 dump: slot 1 holds the firmware
// it shipped with, slot 2 the one it now runs.
func fwPage(afi byte, revs ...string) []byte {
	b := make([]byte, FirmwareSlotSize)
	b[0] = afi
	for i, r := range revs {
		copy(b[8+i*8:16+i*8], []byte(r))
	}
	return b
}

func TestParseFirmwareSlotsActiveAndNext(t *testing.T) {
	f, err := ParseFirmwareSlots(fwPage(0x02, "VDV1DP23", "VDV1DP25"))
	if err != nil {
		t.Fatalf("ParseFirmwareSlots: %v", err)
	}

	if f.Active != 2 {
		t.Errorf("Active = %d, want 2", f.Active)
	}
	if f.Next != 0 {
		t.Errorf("Next = %d, want 0", f.Next)
	}
	if f.Revisions[0] != "VDV1DP23" || f.Revisions[1] != "VDV1DP25" {
		t.Errorf("Revisions = %q", f.Revisions)
	}
}

// Next slot is bits 6:4, active is bits 2:0; on most drives they are equal.
func TestParseFirmwareSlotsNextDiffersFromActive(t *testing.T) {
	f, err := ParseFirmwareSlots(fwPage(0x31, "AAA", "BBB", "CCC"))
	if err != nil {
		t.Fatalf("ParseFirmwareSlots: %v", err)
	}
	if f.Active != 1 {
		t.Errorf("Active = %d, want 1", f.Active)
	}
	if f.Next != 3 {
		t.Errorf("Next = %d, want 3", f.Next)
	}
}

func TestParseFirmwareSlotsSkipsEmptySlots(t *testing.T) {
	f, err := ParseFirmwareSlots(fwPage(0x01, "P8MA002"))
	if err != nil {
		t.Fatalf("ParseFirmwareSlots: %v", err)
	}

	got := f.PopulatedSlots()
	if len(got) != 1 {
		t.Fatalf("PopulatedSlots() returned %d entries, want 1: %+v", len(got), got)
	}
	if got[0].Slot != 1 || got[0].Revision != "P8MA002" {
		t.Errorf("PopulatedSlots()[0] = %+v, want {1 P8MA002}", got[0])
	}
}

// KIOXIA fills three slots with one revision; all three must be reported.
func TestParseFirmwareSlotsIdenticalRevisions(t *testing.T) {
	f, err := ParseFirmwareSlots(fwPage(0x01, "0105", "0105", "0105"))
	if err != nil {
		t.Fatalf("ParseFirmwareSlots: %v", err)
	}
	if got := f.PopulatedSlots(); len(got) != 3 {
		t.Errorf("PopulatedSlots() returned %d entries, want 3: %+v", len(got), got)
	}
}

func TestParseFirmwareSlotsShortBuffer(t *testing.T) {
	if _, err := ParseFirmwareSlots(make([]byte, FirmwareSlotSize-1)); err == nil {
		t.Fatal("err = nil, want an error for a short buffer")
	}
}
