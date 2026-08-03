// Package logpage parses raw NVMe log pages into structs.
package logpage

import "encoding/binary"

// Uint128 is a 128-bit NVMe counter. Float64 is exact only to 2^53 — about
// 9 PB — which is invisible when measuring a rate. Deliberate.
type Uint128 struct {
	Hi uint64
	Lo uint64
}

// Float64 converts Uint128 to float64. See the type comment on precision.
func (u Uint128) Float64() float64 {
	const twoPow64 = 18446744073709551616.0 // 2^64
	return float64(u.Hi)*twoPow64 + float64(u.Lo)
}

// Caller guarantees len(b) >= 16.
func readUint128(b []byte) Uint128 {
	return Uint128{
		Lo: binary.LittleEndian.Uint64(b[0:8]),
		Hi: binary.LittleEndian.Uint64(b[8:16]),
	}
}
