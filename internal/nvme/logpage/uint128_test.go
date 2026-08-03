package logpage

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestUint128Float64(t *testing.T) {
	tests := []struct {
		name string
		in   Uint128
		want float64
	}{
		{"zero", Uint128{}, 0},
		{"low word only", Uint128{Lo: 42}, 42},
		{"carry across 64 bits", Uint128{Hi: 1, Lo: 0}, math.Pow(2, 64)},
		{"max low word", Uint128{Lo: math.MaxUint64}, math.Pow(2, 64) - 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Float64(); got != tt.want {
				t.Errorf("Float64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadUint128LittleEndian(t *testing.T) {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[0:8], 0x1122334455667788)
	binary.LittleEndian.PutUint64(b[8:16], 0x00000000000000AA)

	got := readUint128(b)
	if got.Lo != 0x1122334455667788 {
		t.Errorf("Lo = %#x", got.Lo)
	}
	if got.Hi != 0xAA {
		t.Errorf("Hi = %#x", got.Hi)
	}
}
