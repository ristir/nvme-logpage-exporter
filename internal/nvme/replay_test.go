package nvme

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fakeDumpDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	meta := map[string]any{
		"controllers": []map[string]any{{
			"name":     "nvme0",
			"dev_path": "/dev/nvme0",
			"model":    "SAMSUNG MZVL2512HCJQ-00B07",
			"firmware": "GXA7802Q",
			"serial":   "SYNTHETIC0000002",
		}},
		"namespaces": map[string]any{
			"nvme0": []map[string]any{{
				"name":         "nvme0n1",
				"controller":   "nvme0",
				"size_bytes":   512110190592,
				"sector_bytes": 512,
			}},
		},
	}
	b, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "meta.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	ctrlDir := filepath.Join(root, "nvme0")
	if err := os.MkdirAll(ctrlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	page := make([]byte, 512)
	page[5] = 42 // percentage used, to make it distinguishable from zeros
	if err := os.WriteFile(filepath.Join(ctrlDir, "logpage-0x02.bin"), page, 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestReplayControllersAndNamespaces(t *testing.T) {
	r, err := NewReplay(fakeDumpDir(t))
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	ctrls, err := r.Controllers()
	if err != nil {
		t.Fatalf("Controllers: %v", err)
	}
	if len(ctrls) != 1 || ctrls[0].Serial != "SYNTHETIC0000002" {
		t.Fatalf("Controllers = %+v", ctrls)
	}

	ns, err := r.Namespaces("nvme0")
	if err != nil {
		t.Fatalf("Namespaces: %v", err)
	}
	if len(ns) != 1 || ns[0].SizeBytes != 512110190592 {
		t.Fatalf("Namespaces = %+v", ns)
	}
}

func TestReplayLogPage(t *testing.T) {
	r, err := NewReplay(fakeDumpDir(t))
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	got, err := r.LogPage(context.Background(), "nvme0", 0x02, 512)
	if err != nil {
		t.Fatalf("LogPage: %v", err)
	}
	if len(got) != 512 || got[5] != 42 {
		t.Fatalf("LogPage returned %d bytes, got[5]=%d", len(got), got[5])
	}
}

func TestReplayMissingPageIsUnsupported(t *testing.T) {
	r, err := NewReplay(fakeDumpDir(t))
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	_, err = r.LogPage(context.Background(), "nvme0", 0xC0, 512)
	if !errors.Is(err, ErrPageUnsupported) {
		t.Fatalf("err = %v, want ErrPageUnsupported", err)
	}
}

func TestReplayRejectsMissingDir(t *testing.T) {
	if _, err := NewReplay(filepath.Join(t.TempDir(), "no-such-dir")); err == nil {
		t.Fatal("NewReplay on a nonexistent directory must return an error")
	}
}
