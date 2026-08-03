// Package dump captures raw log page dumps for tests and bug reports.
package dump

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
)

const scrubbedSerial = "SCRUBBED"

// Identify Controller field offsets involved in scrubbing.
const (
	// SN, space-padded ASCII.
	serialOffset = 4
	serialLength = 20

	// SUBNQN. Vendors embed the serial here verbatim — the leak that motivated
	// the whole-buffer scan below.
	subnqnOffset = 768
	subnqnLength = 256

	// Shorter runs risk matching something that merely looks like a serial.
	minSerialScanLength = 6
)

// Unsupported pages are skipped silently.
var pages = []struct {
	id   uint8
	size int
}{
	{0x02, 512},
	{0x01, 512},
	{0x03, 512},
	{0x09, 512},
	{0xC0, 512},
}

type meta struct {
	Controllers []metaController           `json:"controllers"`
	Namespaces  map[string][]metaNamespace `json:"namespaces"`
	MD          map[string]string          `json:"md,omitempty"`
}

type metaController struct {
	Name     string `json:"name"`
	DevPath  string `json:"dev_path"`
	Model    string `json:"model"`
	Firmware string `json:"firmware"`
	Serial   string `json:"serial"`
	State    string `json:"state,omitempty"`
}

type metaNamespace struct {
	Name        string `json:"name"`
	Controller  string `json:"controller"`
	SizeBytes   uint64 `json:"size_bytes"`
	SectorBytes uint64 `json:"sector_bytes"`
}

// Run captures dumps of every controller into outDir. Serials are scrubbed
// by default: dumps get attached to public issues.
func Run(ctx context.Context, src nvme.Source, outDir string, keepSerial bool, logger *slog.Logger) error {
	ctrls, err := src.Controllers()
	if err != nil {
		return fmt.Errorf("enumerating controllers: %w", err)
	}
	if len(ctrls) == 0 {
		return errors.New("no NVMe controllers found")
	}

	m := meta{Namespaces: make(map[string][]metaNamespace)}

	// Host-wide metadata: a failure must not abort the capture, but is logged.
	md, err := src.MDMembership()
	if err != nil {
		logger.Error("failed to read md array membership", "err", err)
		md = map[string]string{}
	}
	m.MD = md

	for _, c := range ctrls {
		dir := filepath.Join(outDir, c.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}

		serial := c.Serial
		if !keepSerial {
			serial = scrubbedSerial
		}

		m.Controllers = append(m.Controllers, metaController{
			Name:     c.Name,
			DevPath:  c.DevPath,
			Model:    c.Model,
			Firmware: c.Firmware,
			Serial:   serial,
			State:    c.State,
		})

		nss, err := src.Namespaces(c.Name)
		if err != nil {
			return fmt.Errorf("namespaces on %s: %w", c.Name, err)
		}
		for _, ns := range nss {
			m.Namespaces[c.Name] = append(m.Namespaces[c.Name], metaNamespace{
				Name:        ns.Name,
				Controller:  ns.Controller,
				SizeBytes:   ns.SizeBytes,
				SectorBytes: ns.SectorBytes,
			})
		}

		if err := dumpIdentify(ctx, src, c.Name, dir, keepSerial, c.Serial); err != nil {
			return err
		}
		if err := dumpPages(ctx, src, c.Name, dir, keepSerial, c.Serial); err != nil {
			return err
		}
	}

	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "meta.json"), append(b, '\n'), 0o644)
}

func dumpIdentify(ctx context.Context, src nvme.Source, ctrl, dir string, keepSerial bool, serial string) error {
	raw, err := src.Identify(ctx, ctrl)
	if errors.Is(err, nvme.ErrPageUnsupported) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading Identify on %s: %w", ctrl, err)
	}

	if !keepSerial {
		scrubbed, err := ScrubIdentify(raw, serial)
		if err != nil {
			return fmt.Errorf("scrubbing Identify on %s: %w", ctrl, err)
		}
		raw = scrubbed
	}

	return os.WriteFile(filepath.Join(dir, "identify.bin"), raw, 0o644)
}

func dumpPages(ctx context.Context, src nvme.Source, ctrl, dir string, keepSerial bool, serial string) error {
	for _, p := range pages {
		raw, err := src.LogPage(ctx, ctrl, p.id, p.size)
		if errors.Is(err, nvme.ErrPageUnsupported) {
			continue
		}
		if err != nil {
			return fmt.Errorf("page %#02x on %s: %w", p.id, ctrl, err)
		}

		if !keepSerial {
			// No known field to blank in a vendor page, so scan the whole buffer.
			raw = scrubSerialFromBuffer(raw, serial)
		}

		name := fmt.Sprintf("logpage-0x%02x.bin", p.id)
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ScrubIdentify blanks the serial three overlapping ways: SN, SUBNQN, and a
// whole-buffer scan. A buffer too short for SUBNQN is refused, not written.
func ScrubIdentify(raw []byte, serial string) ([]byte, error) {
	if len(raw) < subnqnOffset+subnqnLength {
		return nil, fmt.Errorf("identify buffer is %d bytes, need at least %d to scrub the serial and SUBNQN",
			len(raw), subnqnOffset+subnqnLength)
	}

	buf := make([]byte, len(raw))
	copy(buf, raw)

	for i := serialOffset; i < serialOffset+serialLength; i++ {
		buf[i] = ' '
	}
	copy(buf[serialOffset:], scrubbedSerial)

	for i := subnqnOffset; i < subnqnOffset+subnqnLength; i++ {
		buf[i] = 0
	}

	scrubSerialOccurrences(buf, serial)
	return buf, nil
}

// No field layout known, so a whole-buffer scan is all this can do.
func scrubSerialFromBuffer(raw []byte, serial string) []byte {
	buf := make([]byte, len(raw))
	copy(buf, raw)
	scrubSerialOccurrences(buf, serial)
	return buf
}

func scrubSerialOccurrences(buf []byte, serial string) {
	trimmed := strings.TrimSpace(serial)
	if len(trimmed) < minSerialScanLength {
		return
	}

	needle := []byte(trimmed)
	pos := 0
	for {
		idx := bytes.Index(buf[pos:], needle)
		if idx < 0 {
			return
		}
		start := pos + idx
		for i := range needle {
			buf[start+i] = 'X'
		}
		pos = start + len(needle)
	}
}
