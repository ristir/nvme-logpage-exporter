package logpage

import (
	"encoding/binary"
	"errors"
	"testing"
)

func ocpPage(version uint16) []byte {
	b := make([]byte, OCPSmartSize)
	binary.LittleEndian.PutUint16(b[494:496], version)
	copy(b[496:512], ocpGUID[:])
	return b
}

func TestParseOCPSmartRejectsForeignGUID(t *testing.T) {
	b := ocpPage(3)
	for i := 496; i < 512; i++ {
		b[i] = 0
	}

	_, err := ParseOCPSmart(b)
	if !errors.Is(err, ErrNotOCP) {
		t.Fatalf("err = %v, want ErrNotOCP", err)
	}
}

func TestParseOCPSmartShortBuffer(t *testing.T) {
	if _, err := ParseOCPSmart(make([]byte, OCPSmartSize-1)); err == nil {
		t.Fatal("err = nil, want an error for a short buffer")
	}
}

func TestParseOCPSmartOffsets(t *testing.T) {
	b := ocpPage(3)

	binary.LittleEndian.PutUint64(b[0:8], 0x1111)   // phys media written, lo
	binary.LittleEndian.PutUint64(b[16:24], 0x2222) // phys media read, lo
	b[32], b[33] = 0x33, 0x00                       // bad user nand raw
	binary.LittleEndian.PutUint16(b[38:40], 0x44)   // bad user nand normalized
	b[40], b[41] = 0x55, 0x00                       // bad system nand raw
	binary.LittleEndian.PutUint16(b[46:48], 0x66)   // bad system nand normalized
	binary.LittleEndian.PutUint64(b[48:56], 0x77)   // xor recovery
	binary.LittleEndian.PutUint64(b[56:64], 0x88)   // uncorrectable read errors
	binary.LittleEndian.PutUint64(b[64:72], 0x99)   // soft ecc errors
	binary.LittleEndian.PutUint32(b[72:76], 0xAA)   // e2e detected
	binary.LittleEndian.PutUint32(b[76:80], 0xBB)   // e2e corrected
	b[80] = 0xC                                     // system data percent used
	b[81] = 0xD                                     // refresh count, 7 bytes
	binary.LittleEndian.PutUint32(b[88:92], 0xEE)   // max user erase
	binary.LittleEndian.PutUint32(b[92:96], 0xF)    // min user erase
	b[96] = 0x11                                    // thermal throttle events
	b[97] = 0x12                                    // current throttle status
	binary.LittleEndian.PutUint64(b[104:112], 0x13) // pcie correctable errors
	binary.LittleEndian.PutUint32(b[112:116], 0x14) // incomplete shutdowns
	b[120] = 0x15                                   // percent free blocks
	binary.LittleEndian.PutUint16(b[128:130], 0x16) // capacitor health
	binary.LittleEndian.PutUint64(b[136:144], 0x17) // unaligned io
	binary.LittleEndian.PutUint64(b[144:152], 0x18) // security version
	binary.LittleEndian.PutUint64(b[152:160], 0x19) // nuse
	binary.LittleEndian.PutUint64(b[160:168], 0x1A) // plp start count, lo
	binary.LittleEndian.PutUint64(b[176:184], 0x1B) // endurance estimate, lo

	p, err := ParseOCPSmart(b)
	if err != nil {
		t.Fatalf("ParseOCPSmart: %v", err)
	}

	if p.Version != 3 {
		t.Errorf("Version = %d, want 3", p.Version)
	}

	checks := []struct {
		name string
		got  OptU64
		want uint64
	}{
		{"BadUserNANDBlocksRaw", p.BadUserNANDBlocksRaw, 0x33},
		{"BadUserNANDBlocksNormalized", p.BadUserNANDBlocksNormalized, 0x44},
		{"BadSystemNANDBlocksRaw", p.BadSystemNANDBlocksRaw, 0x55},
		{"BadSystemNANDBlocksNormalized", p.BadSystemNANDBlocksNormalized, 0x66},
		{"XORRecoveryCount", p.XORRecoveryCount, 0x77},
		{"UncorrectableReadErrors", p.UncorrectableReadErrors, 0x88},
		{"SoftECCErrors", p.SoftECCErrors, 0x99},
		{"E2EDetectedErrors", p.E2EDetectedErrors, 0xAA},
		{"E2ECorrectedErrors", p.E2ECorrectedErrors, 0xBB},
		{"SystemDataPercentUsed", p.SystemDataPercentUsed, 0xC},
		{"RefreshCount", p.RefreshCount, 0xD},
		{"MaxUserDataEraseCount", p.MaxUserDataEraseCount, 0xEE},
		{"MinUserDataEraseCount", p.MinUserDataEraseCount, 0xF},
		{"ThermalThrottleEvents", p.ThermalThrottleEvents, 0x11},
		{"ThermalThrottleStatusPercent", p.ThermalThrottleStatusPercent, 0x12},
		{"PCIeCorrectableErrors", p.PCIeCorrectableErrors, 0x13},
		{"IncompleteShutdowns", p.IncompleteShutdowns, 0x14},
		{"PercentFreeBlocks", p.PercentFreeBlocks, 0x15},
		{"CapacitorHealth", p.CapacitorHealth, 0x16},
		{"UnalignedIO", p.UnalignedIO, 0x17},
		{"SecurityVersion", p.SecurityVersion, 0x18},
		{"NamespaceUtilizationBytes", p.NamespaceUtilizationBytes, 0x19},
	}
	for _, c := range checks {
		if !c.got.Present {
			t.Errorf("%s: Present = false, want true", c.name)
			continue
		}
		if c.got.Value != c.want {
			t.Errorf("%s = %#x, want %#x", c.name, c.got.Value, c.want)
		}
	}

	wide := []struct {
		name string
		got  OptU128
		want uint64
	}{
		{"PhysicalMediaWrittenBytes", p.PhysicalMediaWrittenBytes, 0x1111},
		{"PhysicalMediaReadBytes", p.PhysicalMediaReadBytes, 0x2222},
		{"PLPStartCount", p.PLPStartCount, 0x1A},
		{"EnduranceEstimateGB", p.EnduranceEstimateGB, 0x1B},
	}
	for _, c := range wide {
		if !c.got.Present {
			t.Errorf("%s: Present = false, want true", c.name)
			continue
		}
		if c.got.Value.Lo != c.want || c.got.Value.Hi != 0 {
			t.Errorf("%s = %+v, want lo=%#x hi=0", c.name, c.got.Value, c.want)
		}
	}
}

func TestParseOCPSmartUnimplementedFieldIsAbsent(t *testing.T) {
	b := ocpPage(3)
	for i := 40; i < 46; i++ {
		b[i] = 0xFF // bad system nand raw, six bytes
	}
	binary.LittleEndian.PutUint16(b[46:48], 0xFFFF) // its normalized value
	b[32] = 0x07                                    // bad user nand raw, real

	p, err := ParseOCPSmart(b)
	if err != nil {
		t.Fatalf("ParseOCPSmart: %v", err)
	}

	if p.BadSystemNANDBlocksRaw.Present {
		t.Error("BadSystemNANDBlocksRaw: Present = true, want false")
	}
	if p.BadSystemNANDBlocksNormalized.Present {
		t.Error("BadSystemNANDBlocksNormalized: Present = true, want false")
	}
	if !p.BadUserNANDBlocksRaw.Present || p.BadUserNANDBlocksRaw.Value != 7 {
		t.Errorf("BadUserNANDBlocksRaw = %+v, want present 7", p.BadUserNANDBlocksRaw)
	}
}

// Versions 2 and 3 have identical layouts on four models, three vendors.
func TestParseOCPSmartAcceptsVersionsTwoAndThree(t *testing.T) {
	for _, v := range []uint16{2, 3} {
		b := ocpPage(v)
		binary.LittleEndian.PutUint64(b[0:8], 4096)

		p, err := ParseOCPSmart(b)
		if err != nil {
			t.Fatalf("version %d: %v", v, err)
		}
		if p.Version != v {
			t.Errorf("version %d: Version = %d", v, p.Version)
		}
		if !p.PhysicalMediaWrittenBytes.Present || p.PhysicalMediaWrittenBytes.Value.Lo != 4096 {
			t.Errorf("version %d: PhysicalMediaWrittenBytes = %+v", v, p.PhysicalMediaWrittenBytes)
		}
	}
}
