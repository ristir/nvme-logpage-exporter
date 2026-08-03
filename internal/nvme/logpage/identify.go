package logpage

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// IdentifySize is the size of the Identify Controller Data Structure.
const IdentifySize = 4096

// Identify is the Identify Controller structure, parsed like a log page.
type Identify struct {
	VendorID uint16
	Serial   string
	Model    string
	Firmware string
	Version  string

	// Reported by the controller itself; zero means unspecified.
	WarnTempKelvin uint16
	CritTempKelvin uint16

	// The maximum supported, not the count: never iterate it.
	MaxNamespaces uint32

	TotalCapacityBytes Uint128
}

// ParseIdentify accepts a longer buffer; a shorter one is an error.
func ParseIdentify(b []byte) (*Identify, error) {
	if len(b) < IdentifySize {
		return nil, fmt.Errorf("Identify Controller: got %d bytes, need %d", len(b), IdentifySize)
	}

	return &Identify{
		VendorID:           binary.LittleEndian.Uint16(b[0:2]),
		Serial:             asciiField(b[4:24]),
		Model:              asciiField(b[24:64]),
		Firmware:           asciiField(b[64:72]),
		Version:            formatVersion(binary.LittleEndian.Uint32(b[80:84])),
		WarnTempKelvin:     binary.LittleEndian.Uint16(b[266:268]),
		CritTempKelvin:     binary.LittleEndian.Uint16(b[268:270]),
		TotalCapacityBytes: readUint128(b[280:296]),
		MaxNamespaces:      binary.LittleEndian.Uint32(b[516:520]),
	}, nil
}

// Trailing NULs matter: without trimming, the serial will not match sysfs.
func asciiField(b []byte) string {
	return strings.TrimRight(strings.TrimRight(string(b), "\x00"), " ")
}

// VER: MJR in bits 31:16, MNR in 15:8, TER in 7:0.
func formatVersion(v uint32) string {
	// "" and not "0.0": zero means the field was never populated.
	if v == 0 {
		return ""
	}
	mjr := (v >> 16) & 0xFFFF
	mnr := (v >> 8) & 0xFF
	ter := v & 0xFF

	s := strconv.FormatUint(uint64(mjr), 10) + "." + strconv.FormatUint(uint64(mnr), 10)
	if ter != 0 {
		s += "." + strconv.FormatUint(uint64(ter), 10)
	}
	return s
}
