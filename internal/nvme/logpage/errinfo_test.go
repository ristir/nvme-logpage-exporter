package logpage

import (
	"encoding/binary"
	"testing"
)

// The only non-empty error log across thirty fleet drives, verbatim.
func realWorldEntry() []byte {
	b := make([]byte, 8*ErrorInfoEntrySize)
	binary.LittleEndian.PutUint64(b[0:8], 178)
	binary.LittleEndian.PutUint16(b[10:12], 12)     // command id
	binary.LittleEndian.PutUint16(b[12:14], 0x4004) // status field
	binary.LittleEndian.PutUint16(b[14:16], 0xFFFF) // parameter error location
	return b
}

// Bit 0 is the phase tag, so the code starts at bit 1 and the type at bit 9.
func TestParseErrorInfoStatusDecoding(t *testing.T) {
	entries, err := ParseErrorInfo(realWorldEntry())
	if err != nil {
		t.Fatalf("ParseErrorInfo: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	e := entries[0]
	if e.Count != 178 {
		t.Errorf("Count = %d, want 178", e.Count)
	}
	if e.StatusCodeType != 0 {
		t.Errorf("StatusCodeType = %d, want 0", e.StatusCodeType)
	}
	if e.StatusCode != 0x02 {
		t.Errorf("StatusCode = %#x, want 0x02", e.StatusCode)
	}
	if !e.More {
		t.Error("More = false, want true")
	}
	if e.DoNotRetry {
		t.Error("DoNotRetry = true, want false")
	}
}

// Every drive ships with a log full of all-zero entries.
func TestParseErrorInfoSkipsEmptyEntries(t *testing.T) {
	b := make([]byte, 8*ErrorInfoEntrySize)
	entries, err := ParseErrorInfo(b)
	if err != nil {
		t.Fatalf("ParseErrorInfo: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestParseErrorInfoDoNotRetryAndSCT(t *testing.T) {
	b := make([]byte, ErrorInfoEntrySize)
	binary.LittleEndian.PutUint64(b[0:8], 1)
	// DNR bit 15, type 1 in bits 11:9, code 0x09 in bits 8:1 = 0x8212.
	binary.LittleEndian.PutUint16(b[12:14], 0x8212)
	binary.LittleEndian.PutUint32(b[24:28], 7)
	binary.LittleEndian.PutUint64(b[16:24], 4096)

	entries, err := ParseErrorInfo(b)
	if err != nil {
		t.Fatalf("ParseErrorInfo: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	e := entries[0]
	if e.StatusCodeType != 1 {
		t.Errorf("StatusCodeType = %d, want 1", e.StatusCodeType)
	}
	if e.StatusCode != 0x09 {
		t.Errorf("StatusCode = %#x, want 0x09", e.StatusCode)
	}
	if !e.DoNotRetry {
		t.Error("DoNotRetry = false, want true")
	}
	if e.NamespaceID != 7 {
		t.Errorf("NamespaceID = %d, want 7", e.NamespaceID)
	}
	if e.LBA != 4096 {
		t.Errorf("LBA = %d, want 4096", e.LBA)
	}
}

func TestParseErrorInfoRejectsPartialEntry(t *testing.T) {
	if _, err := ParseErrorInfo(make([]byte, ErrorInfoEntrySize-1)); err == nil {
		t.Fatal("err = nil, want an error for a buffer shorter than one entry")
	}
}
