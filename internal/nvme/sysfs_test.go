package nvme

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeSysFS(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	ctrl := filepath.Join(root, "class", "nvme", "nvme0")
	if err := os.MkdirAll(ctrl, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(dir, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(ctrl, "model", "SAMSUNG MZVL2512HCJQ-00B07")
	write(ctrl, "firmware_rev", "GXA7802Q")
	write(ctrl, "serial", "SYNTHETIC0000002")

	ns := filepath.Join(root, "block", "nvme0n1")
	if err := os.MkdirAll(filepath.Join(ns, "queue"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(ns, "size", "1000215216")
	// Not 512: sysfs sectors and hw_sector_size must differ or a swap passes.
	write(filepath.Join(ns, "queue"), "hw_sector_size", "4096")

	return root
}

func TestControllersReadsMetadata(t *testing.T) {
	s := SysFS{SysRoot: fakeSysFS(t), DevRoot: "/dev"}

	got, err := s.Controllers()
	if err != nil {
		t.Fatalf("Controllers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d controllers, want 1", len(got))
	}

	c := got[0]
	if c.Name != "nvme0" {
		t.Errorf("Name = %q, want %q", c.Name, "nvme0")
	}
	if c.DevPath != "/dev/nvme0" {
		t.Errorf("DevPath = %q, want %q", c.DevPath, "/dev/nvme0")
	}
	if c.Model != "SAMSUNG MZVL2512HCJQ-00B07" {
		t.Errorf("Model = %q", c.Model)
	}
	if c.Firmware != "GXA7802Q" {
		t.Errorf("Firmware = %q", c.Firmware)
	}
	if c.Serial != "SYNTHETIC0000002" {
		t.Errorf("Serial = %q", c.Serial)
	}
}

func TestControllersEmptyWhenNoNVMe(t *testing.T) {
	s := SysFS{SysRoot: t.TempDir(), DevRoot: "/dev"}

	got, err := s.Controllers()
	if err != nil {
		t.Fatalf("Controllers on a host without NVMe should return an empty list, not an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d controllers, want 0", len(got))
	}
}

func TestNamespacesReadsSizeAndSector(t *testing.T) {
	s := SysFS{SysRoot: fakeSysFS(t), DevRoot: "/dev"}

	got, err := s.Namespaces("nvme0")
	if err != nil {
		t.Fatalf("Namespaces: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d namespaces, want 1", len(got))
	}

	ns := got[0]
	if ns.Name != "nvme0n1" {
		t.Errorf("Name = %q, want %q", ns.Name, "nvme0n1")
	}
	if ns.Controller != "nvme0" {
		t.Errorf("Controller = %q, want %q", ns.Controller, "nvme0")
	}
	if want := uint64(1000215216) * 512; ns.SizeBytes != want {
		t.Errorf("SizeBytes = %d, want %d", ns.SizeBytes, want)
	}
	if ns.SectorBytes != 4096 {
		t.Errorf("SectorBytes = %d, want 4096", ns.SectorBytes)
	}
}

func TestNamespacesIgnoresOtherControllers(t *testing.T) {
	root := fakeSysFS(t)
	// A namespace belonging to a different controller must not appear in the result.
	other := filepath.Join(root, "block", "nvme1n1", "queue")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}

	s := SysFS{SysRoot: root, DevRoot: "/dev"}
	got, err := s.Namespaces("nvme0")
	if err != nil {
		t.Fatalf("Namespaces: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d namespaces, want 1", len(got))
	}
}

func TestControllersMissingAttrIsEmptyString(t *testing.T) {
	root := t.TempDir()
	ctrl := filepath.Join(root, "class", "nvme", "nvme0")
	if err := os.MkdirAll(ctrl, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctrl, "firmware_rev"), []byte("GXA7802Q\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctrl, "serial"), []byte("SYNTHETIC0000002\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := SysFS{SysRoot: root, DevRoot: "/dev"}
	got, err := s.Controllers()
	if err != nil {
		t.Fatalf("Controllers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d controllers, want 1", len(got))
	}
	if got[0].Model != "" {
		t.Errorf("Model = %q, want empty string for missing attribute", got[0].Model)
	}
	if got[0].Firmware != "GXA7802Q" {
		t.Errorf("Firmware = %q, want %q", got[0].Firmware, "GXA7802Q")
	}
}

func TestNamespacesEmptyWhenNoBlockDir(t *testing.T) {
	s := SysFS{SysRoot: t.TempDir(), DevRoot: "/dev"}

	got, err := s.Namespaces("nvme0")
	if err != nil {
		t.Fatalf("Namespaces on a host without /sys/block should return an empty list, not an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d namespaces, want 0", len(got))
	}
}

func TestMDMembership(t *testing.T) {
	root := fakeSysFS(t)

	// /sys/block/md3/md/dev-nvme0n1 marks namespace membership in the array.
	member := filepath.Join(root, "block", "md3", "md", "dev-nvme0n1")
	if err := os.MkdirAll(member, 0o755); err != nil {
		t.Fatal(err)
	}

	s := SysFS{SysRoot: root, DevRoot: "/dev"}
	got, err := s.MDMembership()
	if err != nil {
		t.Fatalf("MDMembership: %v", err)
	}
	if got["nvme0n1"] != "md3" {
		t.Fatalf("MDMembership = %v, want nvme0n1 -> md3", got)
	}
}

func TestMDMembershipEmptyWithoutArrays(t *testing.T) {
	s := SysFS{SysRoot: fakeSysFS(t), DevRoot: "/dev"}

	got, err := s.MDMembership()
	if err != nil {
		t.Fatalf("absence of arrays is not an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

// sysfs really does carry md-prefixed non-arrays: mdadm control files, md_d0.
func TestMDMembershipIgnoresNonArrayMdPrefixedNames(t *testing.T) {
	root := fakeSysFS(t)

	member := filepath.Join(root, "block", "mdadm", "md", "dev-nvme0n1")
	if err := os.MkdirAll(member, 0o755); err != nil {
		t.Fatal(err)
	}

	s := SysFS{SysRoot: root, DevRoot: "/dev"}
	got, err := s.MDMembership()
	if err != nil {
		t.Fatalf("MDMembership: %v", err)
	}
	if _, ok := got["nvme0n1"]; ok {
		t.Fatalf("MDMembership = %v, want nvme0n1 to be absent (\"mdadm\" is not an array name)", got)
	}
}
