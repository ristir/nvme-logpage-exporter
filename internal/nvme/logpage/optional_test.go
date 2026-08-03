package logpage

import "testing"

func TestReadOptUintNormalValue(t *testing.T) {
	b := make([]byte, 16)
	// 0x0102 little-endian at offset 4, two bytes wide.
	b[4], b[5] = 0x02, 0x01

	got := readOptUint(b, 4, 2)
	if !got.Present {
		t.Fatalf("Present = false, want true")
	}
	if got.Value != 0x0102 {
		t.Errorf("Value = %#x, want %#x", got.Value, 0x0102)
	}
}

func TestReadOptUintSentinelIsWidthSpecific(t *testing.T) {
	b := []byte{0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	if got := readOptUint(b, 0, 2); got.Present {
		t.Errorf("two-byte 0xFFFF: Present = true, want false")
	}
	// The same bytes at three bytes wide are 65535, an ordinary value.
	if got := readOptUint(b, 0, 3); !got.Present || got.Value != 65535 {
		t.Errorf("three-byte 0x00FFFF: got %+v, want present 65535", got)
	}

	// The real KIOXIA case: a six-byte field reading 2^48-1 means unimplemented.
	six := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00}
	if got := readOptUint(six, 0, 6); got.Present {
		t.Errorf("six-byte 2^48-1: Present = true, want false")
	}
}

func TestReadOptUintEightByteSentinel(t *testing.T) {
	all := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if got := readOptUint(all, 0, 8); got.Present {
		t.Errorf("eight-byte 2^64-1: Present = true, want false")
	}

	// One bit clear is an ordinary, if enormous, value.
	nearly := []byte{0xFE, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if got := readOptUint(nearly, 0, 8); !got.Present || got.Value != ^uint64(0)-1 {
		t.Errorf("eight-byte 2^64-2: got %+v, want present", got)
	}
}

func TestReadOptUintZeroIsPresent(t *testing.T) {
	b := make([]byte, 8)
	if got := readOptUint(b, 0, 8); !got.Present || got.Value != 0 {
		t.Errorf("got %+v, want present zero", got)
	}
}

func TestReadOptUint128(t *testing.T) {
	all := make([]byte, 16)
	for i := range all {
		all[i] = 0xFF
	}
	if got := readOptUint128(all, 0); got.Present {
		t.Errorf("128-bit all ones: Present = true, want false")
	}

	// Only the high half saturated: a real value, not a sentinel.
	half := make([]byte, 16)
	for i := 8; i < 16; i++ {
		half[i] = 0xFF
	}
	if got := readOptUint128(half, 0); !got.Present {
		t.Errorf("128-bit high half only: Present = false, want true")
	}
}
