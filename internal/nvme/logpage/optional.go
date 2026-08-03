package logpage

// OptU64 is a field a controller may leave unimplemented, which it signals
// with every bit set. Present separates that from a legitimate zero.
type OptU64 struct {
	Value   uint64
	Present bool
}

// OptU128 is the 128-bit counterpart of OptU64.
type OptU128 struct {
	Value   Uint128
	Present bool
}

// Width-specific sentinel: 0xFFFF means absent at two bytes, not at three.
func readOptUint(b []byte, off, n int) OptU64 {
	var v uint64
	for i := n - 1; i >= 0; i-- {
		v = v<<8 | uint64(b[off+i])
	}

	sentinel := ^uint64(0)
	if n < 8 {
		sentinel = 1<<(8*uint(n)) - 1
	}
	if v == sentinel {
		return OptU64{}
	}
	return OptU64{Value: v, Present: true}
}

func readOptUint128(b []byte, off int) OptU128 {
	u := readUint128(b[off : off+16])
	if u.Hi == ^uint64(0) && u.Lo == ^uint64(0) {
		return OptU128{}
	}
	return OptU128{Value: u, Present: true}
}
