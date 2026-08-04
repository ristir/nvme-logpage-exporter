package logpage

import "testing"

func FuzzParseSmart(f *testing.F) {
	f.Add(buildSmartLog())
	f.Add(make([]byte, SmartLogSize))
	f.Add([]byte{})

	full := make([]byte, SmartLogSize)
	for i := range full {
		full[i] = 0xFF
	}
	f.Add(full)

	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = ParseSmart(data)
	})
}

func FuzzParseSelfTest(f *testing.F) {
	f.Add(selfTestPage(0, 0))
	f.Add(selfTestPage(2, 65,
		selfTestEntry(0x27, 3, 0x0F, 0x2edc, 42, 0x1122334455667788, 0x02, 0x81)))
	f.Add(make([]byte, SelfTestSize))
	f.Add([]byte{})

	full := make([]byte, SelfTestSize)
	for i := range full {
		full[i] = 0xFF
	}
	f.Add(full)

	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := ParseSelfTest(data)
		if err != nil {
			return
		}
		if len(s.Results) > selfTestEntries {
			t.Fatalf("Results = %d, more than the page holds", len(s.Results))
		}
		for _, r := range s.Results {
			if r.Result > 0x0F || r.Code > 0x0F {
				t.Fatalf("nibble overflow: result=%d code=%d", r.Result, r.Code)
			}
			if r.Result == resultUnused {
				t.Fatal("an unused entry was reported as a result")
			}
		}
		if s.Completion > 0x7F {
			t.Fatalf("Completion = %d, wider than the seven-bit field", s.Completion)
		}
	})
}
