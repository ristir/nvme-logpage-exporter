package logpage

import (
	"encoding/binary"
	"testing"
)

// Every field gets a value no other field carries, so a shifted offset moves
// a recognisable number.
func selfTestEntry(status, segment, valid byte, poh uint64, nsid uint32, lba uint64, sct, sc byte) []byte {
	e := make([]byte, selfTestEntrySize)
	e[0] = status
	e[1] = segment
	e[2] = valid
	binary.LittleEndian.PutUint64(e[4:12], poh)
	binary.LittleEndian.PutUint32(e[12:16], nsid)
	binary.LittleEndian.PutUint64(e[16:24], lba)
	e[24] = sct
	e[25] = sc
	return e
}

func selfTestPage(inProgress, completion byte, entries ...[]byte) []byte {
	b := make([]byte, SelfTestSize)
	b[0] = inProgress
	b[1] = completion
	// Unwritten slots read as "entry not used".
	for i := 0; i < selfTestEntries; i++ {
		b[4+i*selfTestEntrySize] = resultUnused
	}
	for i, e := range entries {
		copy(b[4+i*selfTestEntrySize:], e)
	}
	return b
}

func TestParseSelfTestDecodesEveryField(t *testing.T) {
	// 0x27: high nibble 2 is an extended test, low nibble 7 is "completed
	// with one or more failed segments".
	page := selfTestPage(0, 0,
		selfTestEntry(0x27, 3, 0x0F, 0x2edc, 42, 0x1122334455667788, 0x02, 0x81))

	s, err := ParseSelfTest(page)
	if err != nil {
		t.Fatalf("ParseSelfTest: %v", err)
	}
	if len(s.Results) != 1 {
		t.Fatalf("Results = %d, want 1", len(s.Results))
	}

	r := s.Results[0]
	for _, c := range []struct {
		name string
		got  uint64
		want uint64
	}{
		{"Result", uint64(r.Result), 7},
		{"Code", uint64(r.Code), 2},
		{"Segment", uint64(r.Segment), 3},
		{"Valid", uint64(r.Valid), 0x0F},
		{"PowerOnHours", r.PowerOnHours, 0x2edc},
		{"NSID", uint64(r.NSID), 42},
		{"FailingLBA", r.FailingLBA, 0x1122334455667788},
		{"StatusType", uint64(r.StatusType), 0x02},
		{"StatusCode", uint64(r.StatusCode), 0x81},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// Reporting unused slots as passes would invent a clean bill of health.
func TestParseSelfTestSkipsUnusedEntries(t *testing.T) {
	s, err := ParseSelfTest(selfTestPage(0, 0))
	if err != nil {
		t.Fatalf("ParseSelfTest: %v", err)
	}
	if len(s.Results) != 0 {
		t.Errorf("Results = %d, want 0", len(s.Results))
	}
}

func TestParseSelfTestRunInProgress(t *testing.T) {
	// Completion is bits 6:0; bit 7 must not leak into the percentage.
	s, err := ParseSelfTest(selfTestPage(0x02, 0xC1))
	if err != nil {
		t.Fatalf("ParseSelfTest: %v", err)
	}
	if s.InProgress != 2 {
		t.Errorf("InProgress = %d, want 2", s.InProgress)
	}
	if s.Completion != 65 {
		t.Errorf("Completion = %d, want 65", s.Completion)
	}
}

func TestSelfTestResultClassification(t *testing.T) {
	for _, c := range []struct {
		result         uint8
		passed, failed bool
	}{
		{0, true, false},  // completed without error
		{1, false, false}, // aborted by a self-test command
		{2, false, false}, // aborted by a controller reset
		{5, false, true},  // fatal error
		{6, false, true},  // unknown failed segment
		{7, false, true},  // one or more failed segments
		{9, false, false}, // aborted by sanitize
	} {
		r := SelfTestResult{Result: c.result}
		if r.Passed() != c.passed {
			t.Errorf("result %d: Passed = %v, want %v", c.result, r.Passed(), c.passed)
		}
		if r.Failed() != c.failed {
			t.Errorf("result %d: Failed = %v, want %v", c.result, r.Failed(), c.failed)
		}
	}
}

func TestParseSelfTestRejectsShortBuffer(t *testing.T) {
	if _, err := ParseSelfTest(make([]byte, SelfTestSize-1)); err == nil {
		t.Fatal("expected an error on a short buffer")
	}
}

// Bytes pinned against a raw page read from a live drive.
func TestParseSelfTestMatchesLiveLayout(t *testing.T) {
	b := selfTestPage(0, 0)
	copy(b[4:], []byte{0x12, 0x00, 0x00, 0x00, 0xdc, 0x2e, 0x00, 0x00})
	s, err := ParseSelfTest(b)
	if err != nil {
		t.Fatalf("ParseSelfTest: %v", err)
	}
	r := s.Results[0]
	if r.Code != 1 || r.Result != 2 || r.PowerOnHours != 0x2edc {
		t.Errorf("got code=%d result=%d poh=%#x, want 1/2/0x2edc", r.Code, r.Result, r.PowerOnHours)
	}
}
