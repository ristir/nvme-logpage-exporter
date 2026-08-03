package dump

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

const fixtureDir = "../../testdata/dumps/synthetic-samsung"

func replaySource(t *testing.T) nvme.Source {
	t.Helper()
	src, err := nvme.NewReplay(fixtureDir)
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	return src
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDumpWritesPagesAndMeta(t *testing.T) {
	out := t.TempDir()

	if err := Run(context.Background(), replaySource(t), out, false, discardLogger()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, name := range []string{
		filepath.Join(out, "meta.json"),
		filepath.Join(out, "nvme0", "identify.bin"),
		filepath.Join(out, "nvme0", "logpage-0x02.bin"),
	} {
		if _, err := os.Stat(name); err != nil {
			t.Errorf("missing file %s: %v", name, err)
		}
	}
}

func TestDumpScrubsSerialByDefault(t *testing.T) {
	out := t.TempDir()

	if err := Run(context.Background(), replaySource(t), out, false, discardLogger()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	meta, err := os.ReadFile(filepath.Join(out, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(meta), "SYNTHETIC0000001") {
		t.Errorf("serial leaked into meta.json:\n%s", meta)
	}

	id, err := os.ReadFile(filepath.Join(out, "nvme0", "identify.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(id[4:24]), "SYNTHETIC") {
		t.Errorf("serial leaked into identify.bin: %q", id[4:24])
	}
	// The model must survive scrubbing; the fixture's differs from meta.json's.
	if !strings.Contains(string(id[24:64]), "IDENTIFY ONLY FALLBACK MODEL") {
		t.Errorf("model was scrubbed, but should not have been: %q", id[24:64])
	}

	// Everything outside 4:24 and 768:1024 must be byte-identical to the source.
	orig, err := os.ReadFile(filepath.Join(fixtureDir, "nvme0", "identify.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != len(orig) {
		t.Fatalf("dumped identify.bin is %d bytes, original is %d bytes", len(id), len(orig))
	}
	if !bytes.Equal(id[:4], orig[:4]) {
		t.Errorf("bytes before the scrub window changed: got %x, want %x", id[:4], orig[:4])
	}
	if !bytes.Equal(id[24:subnqnOffset], orig[24:subnqnOffset]) {
		t.Errorf("bytes between the SN and SUBNQN scrub windows changed")
	}
	if !bytes.Equal(id[subnqnOffset+subnqnLength:], orig[subnqnOffset+subnqnLength:]) {
		t.Errorf("bytes after the SUBNQN scrub window changed")
	}
}

func TestDumpKeepsSerialWhenAsked(t *testing.T) {
	out := t.TempDir()

	if err := Run(context.Background(), replaySource(t), out, true, discardLogger()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var meta struct {
		Controllers []struct {
			Serial string `json:"serial"`
		} `json:"controllers"`
	}
	b, err := os.ReadFile(filepath.Join(out, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.Controllers) != 1 || meta.Controllers[0].Serial != "SYNTHETIC0000001" {
		t.Errorf("with --keep-serial the serial must be preserved: %+v", meta.Controllers)
	}

	// --keep-serial must spare the binary Identify too, not only meta.json.
	id, err := os.ReadFile(filepath.Join(out, "nvme0", "identify.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(id[4:24]), "SYNTHETIC0000001") {
		t.Errorf("with --keep-serial the serial must be preserved in identify.bin: %q", id[4:24])
	}
}

func TestDumpRoundTripsThroughReplay(t *testing.T) {
	out := t.TempDir()

	if err := Run(context.Background(), replaySource(t), out, false, discardLogger()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	replayed, err := nvme.NewReplay(out)
	if err != nil {
		t.Fatalf("NewReplay on dump's own output: %v", err)
	}

	ctrls, err := replayed.Controllers()
	if err != nil {
		t.Fatalf("Controllers: %v", err)
	}
	if len(ctrls) != 1 {
		t.Fatalf("got %d controllers, want 1", len(ctrls))
	}
	if ctrls[0].Model != "SAMSUNG MZVL2512HCJQ-00B07" {
		t.Errorf("Model = %q", ctrls[0].Model)
	}
	if ctrls[0].Firmware != "GXA7802Q" {
		t.Errorf("Firmware = %q", ctrls[0].Firmware)
	}
	if ctrls[0].Serial != scrubbedSerial {
		t.Errorf("Serial = %q, want %q (scrubbed by default)", ctrls[0].Serial, scrubbedSerial)
	}

	md, err := replayed.MDMembership()
	if err != nil {
		t.Fatalf("MDMembership on the round-tripped dump: %v", err)
	}
	if want := (map[string]string{"nvme0n1": "md3"}); !maps.Equal(md, want) {
		t.Errorf("MDMembership = %v, want %v", md, want)
	}

	raw, err := replayed.LogPage(context.Background(), "nvme0", logpage.IDSmart, logpage.SmartLogSize)
	if err != nil {
		t.Fatalf("LogPage 0x02 on the round-tripped dump: %v", err)
	}
	if _, err := logpage.ParseSmart(raw); err != nil {
		t.Errorf("ParseSmart on the round-tripped page: %v", err)
	}
}

type fakeSource struct {
	identify []byte
	serial   string
	logPage  []byte
}

func (f *fakeSource) Controllers() ([]nvme.Controller, error) {
	serial := f.serial
	if serial == "" {
		serial = "FAKESERIAL0000001"
	}
	return []nvme.Controller{{Name: "nvme0", DevPath: "/dev/nvme0", Model: "FAKE MODEL", Firmware: "FAKEFW1", Serial: serial}}, nil
}

func (f *fakeSource) Namespaces(string) ([]nvme.Namespace, error) { return nil, nil }

func (f *fakeSource) LogPage(context.Context, string, uint8, int) ([]byte, error) {
	if f.logPage == nil {
		return nil, nvme.ErrPageUnsupported
	}
	return f.logPage, nil
}

func (f *fakeSource) Identify(context.Context, string) ([]byte, error) {
	return f.identify, nil
}

func (f *fakeSource) MDMembership() (map[string]string, error) { return map[string]string{}, nil }

func fakeIdentify(serial, subnqn string, withVendorRegionLeak bool) []byte {
	b := make([]byte, 4096)
	copy(b[serialOffset:], serial)
	for i := len(serial); i < serialLength; i++ {
		b[serialOffset+i] = ' '
	}
	copy(b[24:64], "FAKE MODEL")
	copy(b[subnqnOffset:], subnqn)
	if withVendorRegionLeak {
		copy(b[3072:], serial)
	}
	return b
}

func TestScrubIdentifyBlanksSubNQN(t *testing.T) {
	serial := "REALSERIAL000001"
	raw := fakeIdentify(serial, "nqn.2016-08.com.example:nvme:nvm-subsystem-sn-"+serial, false)

	scrubbed, err := ScrubIdentify(raw, serial)
	if err != nil {
		t.Fatalf("ScrubIdentify: %v", err)
	}

	subnqn := scrubbed[subnqnOffset : subnqnOffset+subnqnLength]
	for i, b := range subnqn {
		if b != 0 {
			t.Fatalf("SUBNQN byte %d = %#x, want 0 (fully blanked): %q", i, b, subnqn)
		}
	}
	if bytes.Contains(scrubbed, []byte(serial)) {
		t.Errorf("serial %q still present in scrubbed buffer", serial)
	}
	if !bytes.Contains(scrubbed[24:64], []byte("FAKE MODEL")) {
		t.Errorf("model was scrubbed, but should not have been: %q", scrubbed[24:64])
	}
}

func TestScrubIdentifyCatchesVendorRegionSerial(t *testing.T) {
	serial := "REALSERIAL000001"
	raw := fakeIdentify(serial, "", true) // no SUBNQN; leak lives at byte 3072 instead

	scrubbed, err := ScrubIdentify(raw, serial)
	if err != nil {
		t.Fatalf("ScrubIdentify: %v", err)
	}
	if bytes.Contains(scrubbed, []byte(serial)) {
		t.Errorf("serial %q leaked through a vendor-specific region the scrubber does not know about: %q",
			serial, scrubbed[3072:3072+len(serial)])
	}
}

func TestScrubIdentifySkipsShortSerial(t *testing.T) {
	raw := fakeIdentify("AB1", "", false)
	copy(raw[3072:], "AB1 elsewhere too")

	scrubbed, err := ScrubIdentify(raw, "AB1")
	if err != nil {
		t.Fatalf("ScrubIdentify: %v", err)
	}
	if !bytes.Contains(scrubbed[3072:], []byte("AB1 elsewhere too")) {
		t.Errorf("a serial shorter than the minimum scan length must not trigger the whole-buffer scan, but bytes at 3072 changed: %q", scrubbed[3072:3072+20])
	}
}

func TestScrubIdentifyErrorsOnBufferTooShortForSubNQN(t *testing.T) {
	if _, err := ScrubIdentify(make([]byte, 512), "SOMESERIAL01"); err == nil {
		t.Fatal("ScrubIdentify: want an error for a buffer too short to reach SUBNQN, got nil")
	}
}

func TestDumpScrubsSerialFromLogPages(t *testing.T) {
	out := t.TempDir()
	serial := "REALSERIAL000001"

	page := make([]byte, 512)
	copy(page[100:], "leading junk "+serial+" trailing junk")

	src := &fakeSource{
		identify: fakeIdentify(serial, "", false),
		serial:   serial,
		logPage:  page,
	}

	if err := Run(context.Background(), src, out, false, discardLogger()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "nvme0", "logpage-0x02.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte(serial)) {
		t.Errorf("serial %q leaked into a captured log page: %q", serial, got)
	}
}

func TestDumpErrorsOnShortIdentifyBuffer(t *testing.T) {
	out := t.TempDir()
	src := &fakeSource{identify: make([]byte, 10)} // shorter than the 24-byte scrub window

	if err := Run(context.Background(), src, out, false, discardLogger()); err == nil {
		t.Fatal("Run: want an error for an Identify buffer too short to scrub, got nil")
	}
}
