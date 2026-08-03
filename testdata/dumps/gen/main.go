//go:build ignore

// Synthetic dump generator: go run testdata/dumps/gen/main.go
package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
)

func main() {
	b := make([]byte, 512)
	b[0] = 0x05
	binary.LittleEndian.PutUint16(b[1:3], 304)
	b[3], b[4], b[5] = 100, 10, 2

	put := func(off int, v uint64) { binary.LittleEndian.PutUint64(b[off:off+8], v) }
	put(32, 17122402)
	put(48, 26169783)
	put(64, 252898188)
	put(80, 386547283)
	put(96, 1448)
	put(112, 89)
	put(128, 1844)
	put(144, 65)

	binary.LittleEndian.PutUint32(b[192:196], 7)
	binary.LittleEndian.PutUint32(b[196:200], 3)
	binary.LittleEndian.PutUint16(b[200:202], 304)
	binary.LittleEndian.PutUint16(b[202:204], 303)

	dir := filepath.Join("testdata", "dumps", "synthetic-samsung", "nvme0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logpage-0x02.bin"), b, 0o644); err != nil {
		panic(err)
	}

	id := make([]byte, 4096)
	binary.LittleEndian.PutUint16(id[0:2], 0x144D)
	copy(id[4:24], []byte("SYNTHETIC0000001    "))
	// Deliberately unlike meta.json's model, so a test can tell the two apart.
	copy(id[24:64], []byte("IDENTIFY ONLY FALLBACK MODEL"))
	copy(id[64:72], []byte("GXA7802Q"))
	binary.LittleEndian.PutUint32(id[80:84], 0x00010300)
	binary.LittleEndian.PutUint16(id[266:268], 357)
	binary.LittleEndian.PutUint16(id[268:270], 358)
	// Deliberately unlike the namespace size, so a swapped source is visible.
	binary.LittleEndian.PutUint64(id[280:288], 549755813888)
	// 128 against one real namespace, as seen on the fleet: a swap is visible.
	binary.LittleEndian.PutUint32(id[516:520], 128)

	if err := os.WriteFile(filepath.Join(dir, "identify.bin"), id, 0o644); err != nil {
		panic(err)
	}
}
