//go:build ignore

// Command gen_ocp writes the synthetic-ocp fixture: non-zero throttling and
// all-ones fields, neither of which any fleet hardware exhibits.
package main

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

const outDir = "testdata/dumps/synthetic-ocp"

func main() {
	if err := os.MkdirAll(filepath.Join(outDir, "nvme0"), 0o755); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(outDir, "nvme0", "logpage-0xc0.bin"), ocpPage(), 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "nvme0", "logpage-0x02.bin"), smartPage(), 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "nvme0", "identify.bin"), identify(), 0o644); err != nil {
		log.Fatal(err)
	}

	meta := map[string]any{
		"controllers": []map[string]string{{
			"name":     "nvme0",
			"dev_path": "/dev/nvme0",
			"model":    "SYNTHETIC OCP TESTDEV",
			"firmware": "0001",
			"serial":   "SCRUBBED",
		}},
		"namespaces": map[string]any{
			"nvme0": []map[string]any{{
				"name":         "nvme0n1",
				"controller":   "nvme0",
				"size_bytes":   1000204886016,
				"sector_bytes": 512,
			}},
		},
	}
	buf, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "meta.json"), append(buf, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
}

func ocpPage() []byte {
	b := make([]byte, 512)

	binary.LittleEndian.PutUint64(b[0:8], 1<<50) // physical media written
	binary.LittleEndian.PutUint64(b[16:24], 1<<49)

	b[32] = 3                                   // bad user nand raw
	binary.LittleEndian.PutUint16(b[38:40], 99) // bad user nand normalized
	for i := 40; i < 48; i++ {                  // bad system nand: not implemented
		b[i] = 0xFF
	}

	b[96] = 7  // thermal throttling events
	b[97] = 25 // current throttling status, percent

	for i := 128; i < 130; i++ { // capacitor health: not implemented
		b[i] = 0xFF
	}
	for i := 144; i < 152; i++ { // security version: not implemented
		b[i] = 0xFF
	}

	binary.LittleEndian.PutUint16(b[494:496], 3)
	copy(b[496:512], []byte{
		0xc5, 0xaf, 0x10, 0x28, 0xea, 0xbf, 0xf2, 0xa4,
		0x9c, 0x4f, 0x6f, 0x7c, 0xc9, 0x14, 0xd5, 0xaf,
	})
	return b
}

func smartPage() []byte {
	b := make([]byte, 512)
	binary.LittleEndian.PutUint16(b[1:3], 313) // composite temperature, K
	b[3] = 100                                 // available spare
	b[4] = 10                                  // available spare threshold
	b[5] = 12                                  // percentage used
	binary.LittleEndian.PutUint16(b[200:202], 311)
	return b
}

func identify() []byte {
	b := make([]byte, 4096)
	copy(b[4:24], []byte("SCRUBBED            "))
	copy(b[24:64], []byte("SYNTHETIC OCP TESTDEV                   "))
	copy(b[64:72], []byte("0001    "))
	binary.LittleEndian.PutUint16(b[266:268], 358) // WCTEMP
	binary.LittleEndian.PutUint16(b[268:270], 361) // CCTEMP
	binary.LittleEndian.PutUint32(b[516:520], 1)   // NN
	return b
}
