package logpage

import (
	"encoding/binary"
	"fmt"
)

// IDErrorInfo is the Error Information log page.
const IDErrorInfo uint8 = 0x01

// ErrorInfoEntrySize is fixed; the page holds ELPE+1 entries.
const ErrorInfoEntrySize = 64

// ErrorEntry decodes only fields verifiable against real hardware; the rest
// is left alone rather than guessed from the spec.
type ErrorEntry struct {
	Count uint64

	// Bit 0 is the phase tag, so both sit one bit higher than a naive read.
	StatusCodeType uint8
	StatusCode     uint8
	More           bool
	DoNotRetry     bool

	NamespaceID uint32
	LBA         uint64
}

// ParseErrorInfo skips all-zero entries: decoding one reports a success that
// never happened.
func ParseErrorInfo(b []byte) ([]ErrorEntry, error) {
	if len(b) < ErrorInfoEntrySize {
		return nil, fmt.Errorf("page 0x01: got %d bytes, need at least %d", len(b), ErrorInfoEntrySize)
	}

	var out []ErrorEntry
	for off := 0; off+ErrorInfoEntrySize <= len(b); off += ErrorInfoEntrySize {
		e := b[off : off+ErrorInfoEntrySize]
		if allZero(e) {
			continue
		}

		status := binary.LittleEndian.Uint16(e[12:14])
		out = append(out, ErrorEntry{
			Count:          binary.LittleEndian.Uint64(e[0:8]),
			StatusCodeType: uint8(status >> 9 & 0x07),
			StatusCode:     uint8(status >> 1 & 0xFF),
			More:           status>>14&1 == 1,
			DoNotRetry:     status>>15&1 == 1,
			LBA:            binary.LittleEndian.Uint64(e[16:24]),
			NamespaceID:    binary.LittleEndian.Uint32(e[24:28]),
		})
	}
	return out, nil
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}
