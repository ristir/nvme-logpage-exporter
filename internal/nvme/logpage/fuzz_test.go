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
