package logpage

import (
	"encoding/binary"
	"testing"
)

func buildIdentify() []byte {
	b := make([]byte, IdentifySize)

	binary.LittleEndian.PutUint16(b[0:2], 0x144D) // Samsung
	copy(b[4:24], []byte("SYNTHETIC0000002    "))
	copy(b[24:64], []byte("SAMSUNG MZVL2512HCJQ-00B07              "))
	copy(b[64:72], []byte("GXA7802Q"))
	binary.LittleEndian.PutUint32(b[80:84], 0x00010300)     // 1.3.0
	binary.LittleEndian.PutUint16(b[266:268], 357)          // WCTEMP, ~84 C
	binary.LittleEndian.PutUint16(b[268:270], 358)          // CCTEMP, ~85 C
	binary.LittleEndian.PutUint64(b[280:288], 512110190592) // TNVMCAP low 8 bytes
	// A non-zero TNVMCAP high half catches a Hi/Lo transposition.
	binary.LittleEndian.PutUint64(b[288:296], 7)
	binary.LittleEndian.PutUint32(b[516:520], 1)

	return b
}

func TestParseIdentifyFields(t *testing.T) {
	got, err := ParseIdentify(buildIdentify())
	if err != nil {
		t.Fatalf("ParseIdentify: %v", err)
	}

	if got.VendorID != 0x144D {
		t.Errorf("VendorID = %#x, want 0x144d", got.VendorID)
	}
	// ASCII fields are space-padded; untrimmed, the label will not match sysfs.
	if got.Serial != "SYNTHETIC0000002" {
		t.Errorf("Serial = %q", got.Serial)
	}
	if got.Model != "SAMSUNG MZVL2512HCJQ-00B07" {
		t.Errorf("Model = %q", got.Model)
	}
	if got.Firmware != "GXA7802Q" {
		t.Errorf("Firmware = %q", got.Firmware)
	}
	if got.Version != "1.3" {
		t.Errorf("Version = %q, want 1.3", got.Version)
	}
	if got.WarnTempKelvin != 357 || got.CritTempKelvin != 358 {
		t.Errorf("thresholds = %d/%d, want 357/358", got.WarnTempKelvin, got.CritTempKelvin)
	}
	if got.TotalCapacityBytes.Lo != 512110190592 || got.TotalCapacityBytes.Hi != 7 {
		t.Errorf("TotalCapacityBytes = {Hi:%d Lo:%d}, want {Hi:7 Lo:512110190592}",
			got.TotalCapacityBytes.Hi, got.TotalCapacityBytes.Lo)
	}
	if got.MaxNamespaces != 1 {
		t.Errorf("MaxNamespaces = %d, want 1", got.MaxNamespaces)
	}
}

func TestParseIdentifyVersionWithTertiary(t *testing.T) {
	b := buildIdentify()
	binary.LittleEndian.PutUint32(b[80:84], 0x00010401) // 1.4.1

	got, err := ParseIdentify(b)
	if err != nil {
		t.Fatalf("ParseIdentify: %v", err)
	}
	if got.Version != "1.4.1" {
		t.Errorf("Version = %q, want 1.4.1", got.Version)
	}
}

// Real controllers do report VER=0; "0.0" would read as a real version.
func TestParseIdentifyZeroVersionIsEmptyString(t *testing.T) {
	b := buildIdentify()
	binary.LittleEndian.PutUint32(b[80:84], 0)

	got, err := ParseIdentify(b)
	if err != nil {
		t.Fatalf("ParseIdentify: %v", err)
	}
	if got.Version != "" {
		t.Errorf("Version = %q, want empty string for VER=0", got.Version)
	}
}

func TestParseIdentifyRejectsShortBuffer(t *testing.T) {
	for _, n := range []int{0, 519, 4095} {
		if _, err := ParseIdentify(make([]byte, n)); err == nil {
			t.Errorf("ParseIdentify(%d bytes): no error, but one is expected", n)
		}
	}
}

func FuzzParseIdentify(f *testing.F) {
	f.Add(buildIdentify())
	f.Add([]byte{})
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = ParseIdentify(data)
	})
}
