package nvme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

// IdentifySize aliases logpage.IdentifySize so callers skip that import.
const IdentifySize = logpage.IdentifySize

type replayMeta struct {
	Controllers []replayController           `json:"controllers"`
	Namespaces  map[string][]replayNamespace `json:"namespaces"`
	MD          map[string]string            `json:"md,omitempty"`
}

type replayController struct {
	Name     string `json:"name"`
	DevPath  string `json:"dev_path"`
	Model    string `json:"model"`
	Firmware string `json:"firmware"`
	Serial   string `json:"serial"`
	State    string `json:"state,omitempty"`
}

type replayNamespace struct {
	Name        string `json:"name"`
	Controller  string `json:"controller"`
	SizeBytes   uint64 `json:"size_bytes"`
	SectorBytes uint64 `json:"sector_bytes"`
}

// Reads pre-captured dumps instead of a real device.
type replaySource struct {
	dir  string
	meta replayMeta
}

// NewReplay opens a dump directory in the format CONTRIBUTING.md describes.
func NewReplay(dir string) (Source, error) {
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, fmt.Errorf("dump directory %s: %w", dir, err)
	}

	var m replayMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("meta.json in %s: %w", dir, err)
	}

	return &replaySource{dir: dir, meta: m}, nil
}

func (r *replaySource) Controllers() ([]Controller, error) {
	out := make([]Controller, 0, len(r.meta.Controllers))
	for _, c := range r.meta.Controllers {
		out = append(out, Controller(c))
	}
	return out, nil
}

func (r *replaySource) Namespaces(controller string) ([]Namespace, error) {
	src := r.meta.Namespaces[controller]
	out := make([]Namespace, 0, len(src))
	for _, n := range src {
		out = append(out, Namespace(n))
	}
	return out, nil
}

func (r *replaySource) LogPage(_ context.Context, controller string, pageID uint8, _ int) ([]byte, error) {
	name := fmt.Sprintf("logpage-0x%02x.bin", pageID)
	return r.readBin(controller, name)
}

func (r *replaySource) Identify(_ context.Context, controller string) ([]byte, error) {
	return r.readBin(controller, "identify.bin")
}

func (r *replaySource) MDMembership() (map[string]string, error) {
	if r.meta.MD == nil {
		return map[string]string{}, nil
	}
	return r.meta.MD, nil
}

func (r *replaySource) readBin(controller, name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(r.dir, controller, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s/%s: %w", controller, name, ErrPageUnsupported)
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

var _ Source = (*replaySource)(nil)
