package nvme

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// SysFS reads sysfs; the roots are fields so tests can point them at TempDir.
type SysFS struct {
	SysRoot string // "/sys"
	DevRoot string // "/dev"
}

// NewSysFS returns a SysFS pointed at the real host roots.
func NewSysFS() SysFS {
	return SysFS{SysRoot: "/sys", DevRoot: "/dev"}
}

var (
	controllerRe = regexp.MustCompile(`^nvme[0-9]+$`)
	namespaceRe  = regexp.MustCompile(`^nvme[0-9]+n[0-9]+$`)
	mdArrayRe    = regexp.MustCompile(`^md[0-9]+$`)
)

// Controllers returns nothing, not an error, when there is no NVMe here.
func (s SysFS) Controllers() ([]Controller, error) {
	dir := filepath.Join(s.SysRoot, "class", "nvme")

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var out []Controller
	for _, e := range entries {
		if !controllerRe.MatchString(e.Name()) {
			continue
		}
		base := filepath.Join(dir, e.Name())
		out = append(out, Controller{
			Name:     e.Name(),
			DevPath:  filepath.Join(s.DevRoot, e.Name()),
			Model:    readAttr(base, "model"),
			Firmware: readAttr(base, "firmware_rev"),
			Serial:   readAttr(base, "serial"),
			State:    readAttr(base, "state"),
		})
	}
	return out, nil
}

// Namespaces walks the block devices that exist: NN is a maximum, not a count.
func (s SysFS) Namespaces(controller string) ([]Namespace, error) {
	dir := filepath.Join(s.SysRoot, "block")

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	prefix := controller + "n"

	var out []Namespace
	for _, e := range entries {
		name := e.Name()
		if !namespaceRe.MatchString(name) || !strings.HasPrefix(name, prefix) {
			continue
		}

		base := filepath.Join(dir, name)
		ns := Namespace{Name: name, Controller: controller}

		// sysfs reports 512-byte sectors regardless of the logical block size.
		if sectors, ok := readUint(base, "size"); ok {
			ns.SizeBytes = sectors * 512
		}
		if hw, ok := readUint(filepath.Join(base, "queue"), "hw_sector_size"); ok {
			ns.SectorBytes = hw
		}

		out = append(out, ns)
	}
	return out, nil
}

// MDMembership returns an empty map, not an error, when there are no arrays.
func (s SysFS) MDMembership() (map[string]string, error) {
	dir := filepath.Join(s.SysRoot, "block")

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}

	out := make(map[string]string)
	for _, e := range entries {
		name := e.Name()
		if !mdArrayRe.MatchString(name) {
			continue
		}

		members, err := os.ReadDir(filepath.Join(dir, name, "md"))
		if err != nil {
			continue
		}
		for _, m := range members {
			dev, ok := strings.CutPrefix(m.Name(), "dev-")
			if !ok || !namespaceRe.MatchString(dev) {
				continue
			}
			out[dev] = name
		}
	}
	return out, nil
}

func readAttr(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readUint(dir, name string) (uint64, bool) {
	v := readAttr(dir, name)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
