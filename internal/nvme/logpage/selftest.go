package logpage

import (
	"encoding/binary"
	"fmt"
)

// IDSelfTest is the Device Self-test log page.
const IDSelfTest uint8 = 0x06

// SelfTestSize is the fixed size of page 0x06.
const SelfTestSize = 4 + selfTestEntries*selfTestEntrySize

const (
	selfTestEntries   = 20
	selfTestEntrySize = 28
)

// A drive that has run no self-test returns twenty of these, which is not the
// same as twenty passes.
const resultUnused = 0x0F

// SelfTestResult is one entry of the log.
type SelfTestResult struct {
	Result uint8 // low nibble of the status byte
	Code   uint8 // high nibble: 1 short, 2 extended, 0xE vendor specific

	Segment      uint8
	PowerOnHours uint64

	// Only meaningful when the matching bit of Valid is set: a zero LBA is a
	// real address, so absence cannot be read off the value.
	Valid      uint8
	NSID       uint32
	FailingLBA uint64
	StatusType uint8
	StatusCode uint8
}

// SelfTest is the parsed page 0x06.
type SelfTest struct {
	InProgress uint8 // 0 when idle
	Completion uint8 // percent, only meaningful while a test runs

	// Newest first, and unused slots are dropped.
	Results []SelfTestResult
}

// ParseSelfTest parses page 0x06.
func ParseSelfTest(b []byte) (*SelfTest, error) {
	if len(b) < SelfTestSize {
		return nil, fmt.Errorf("self-test log: got %d bytes, want %d", len(b), SelfTestSize)
	}

	s := &SelfTest{
		InProgress: b[0] & 0x0F,
		Completion: b[1] & 0x7F,
	}

	for i := 0; i < selfTestEntries; i++ {
		e := b[4+i*selfTestEntrySize:]
		status := e[0]
		if status&0x0F == resultUnused {
			continue
		}
		s.Results = append(s.Results, SelfTestResult{
			Result:       status & 0x0F,
			Code:         status >> 4,
			Segment:      e[1],
			Valid:        e[2],
			PowerOnHours: binary.LittleEndian.Uint64(e[4:12]),
			NSID:         binary.LittleEndian.Uint32(e[12:16]),
			FailingLBA:   binary.LittleEndian.Uint64(e[16:24]),
			StatusType:   e[24],
			StatusCode:   e[25],
		})
	}

	return s, nil
}

// Passed reports whether the run completed without error. Aborted runs are
// neither passes nor failures: the drive stopped for a reason outside the
// medium, most often a controller reset.
func (r SelfTestResult) Passed() bool { return r.Result == 0 }

// Failed reports whether the drive found a defect, as opposed to giving up.
func (r SelfTestResult) Failed() bool {
	switch r.Result {
	case 5, 6, 7:
		return true
	}
	return false
}
