package collector

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
)

type fakeSource struct {
	controller nvme.Controller
	namespaces []nvme.Namespace
	identify   []byte

	smartRaw []byte
}

func (f fakeSource) Controllers() ([]nvme.Controller, error) {
	return []nvme.Controller{f.controller}, nil
}
func (f fakeSource) Namespaces(string) ([]nvme.Namespace, error) { return f.namespaces, nil }
func (f fakeSource) LogPage(_ context.Context, _ string, _ uint8, _ int) ([]byte, error) {
	if f.smartRaw == nil {
		return nil, nvme.ErrPageUnsupported
	}
	return f.smartRaw, nil
}
func (f fakeSource) Identify(context.Context, string) ([]byte, error) { return f.identify, nil }
func (f fakeSource) MDMembership() (map[string]string, error)         { return map[string]string{}, nil }

var _ nvme.Source = fakeSource{}

func buildRawIdentify(model, firmware string, nn uint32) []byte {
	b := make([]byte, nvme.IdentifySize)
	copy(b[24:64], []byte(model))
	copy(b[64:72], []byte(firmware))
	binary.LittleEndian.PutUint32(b[516:520], nn)
	return b
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDeviceInfoMetric(t *testing.T) {
	e := testExporter(t)

	out := gatherText(t, e, "nvme_logpage_device_info")

	for _, s := range []string{
		`device="nvme0"`,
		`serial="SYNTHETIC0000001"`,
		`model="SAMSUNG MZVL2512HCJQ-00B07"`,
		`firmware="GXA7802Q"`,
		`nvme_version="1.3"`,
		`vendor_id="0x144d"`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("nvme_logpage_device_info is missing %q:\n%s", s, out)
		}
	}
	// Match the line terminator too: "1" is a suffix of "11" and "0.1".
	if !strings.Contains(out, "} 1\n") {
		t.Errorf("info metric value must be 1:\n%s", out)
	}
}

// The fixture's identify.bin model deliberately differs from meta.json's.
func TestDeviceInfoPrefersSysfsModelWhenPresent(t *testing.T) {
	e := testExporter(t)

	out := gatherText(t, e, "nvme_logpage_device_info")

	if !strings.Contains(out, `model="SAMSUNG MZVL2512HCJQ-00B07"`) {
		t.Errorf("sysfs model should win when sysfs has one:\n%s", out)
	}
	if strings.Contains(out, "IDENTIFY ONLY FALLBACK MODEL") {
		t.Errorf("Identify's model leaked through even though sysfs had one:\n%s", out)
	}
}

// On one fleet device sysfs knew the model while the device returned nothing.
func TestDeviceInfoFallsBackToIdentifyWhenSysfsEmpty(t *testing.T) {
	src := fakeSource{
		controller: nvme.Controller{Name: "nvme0", Serial: "SYNTHETIC0000099"},
		identify:   buildRawIdentify("IDENTIFY ONLY FALLBACK MODEL", "IDFWFAKE", 1),
	}
	e := New(src, discardLogger())

	out := gatherText(t, e, "nvme_logpage_device_info")

	if !strings.Contains(out, `model="IDENTIFY ONLY FALLBACK MODEL"`) {
		t.Errorf("empty sysfs model should fall back to Identify's model:\n%s", out)
	}
	if !strings.Contains(out, `firmware="IDFWFAKE"`) {
		t.Errorf("empty sysfs firmware should fall back to Identify's firmware:\n%s", out)
	}
}

func TestDeviceCapacityMetric(t *testing.T) {
	e := testExporter(t)

	out := gatherText(t, e, "nvme_logpage_capacity_bytes")
	// TNVMCAP deliberately differs from the namespace size, so a swap fails.
	if !strings.Contains(out, "} 5.49755813888e+11\n") {
		t.Errorf("capacity_bytes:\n%s", out)
	}
}

func TestNamespaceCountsAreSeparateMetrics(t *testing.T) {
	e := testExporter(t)

	maxOut := gatherText(t, e, "nvme_logpage_namespaces_max")
	activeOut := gatherText(t, e, "nvme_logpage_namespaces_active")

	if !strings.Contains(maxOut, "} 128\n") {
		t.Errorf("namespaces_max:\n%s", maxOut)
	}
	if !strings.Contains(activeOut, "} 2\n") {
		t.Errorf("namespaces_active:\n%s", activeOut)
	}
}

func TestNamespacesMaxNotEmittedWhenControllerReportsNone(t *testing.T) {
	src := fakeSource{
		controller: nvme.Controller{Name: "nvme0", Serial: "SYNTHETIC0000098"},
		identify:   buildRawIdentify("", "", 0),
	}
	e := New(src, discardLogger())

	out := gatherText(t, e, "nvme_logpage_namespaces_max")
	if out != "" {
		t.Errorf("NN=0 must not be emitted as namespaces_max:\n%s", out)
	}
}

func TestTemperatureThresholdsFromHardware(t *testing.T) {
	e := testExporter(t)

	warn := gatherText(t, e, "nvme_logpage_composite_temperature_warning_threshold_celsius")
	crit := gatherText(t, e, "nvme_logpage_composite_temperature_critical_threshold_celsius")

	// 357 K - 273.15 = 83.85; 358 K - 273.15 = 84.85
	if !strings.Contains(warn, "83.85") {
		t.Errorf("warning threshold:\n%s", warn)
	}
	if !strings.Contains(crit, "84.85") {
		t.Errorf("critical threshold:\n%s", crit)
	}
}

func TestNamespaceMetrics(t *testing.T) {
	e := testExporter(t)

	size := gatherText(t, e, "nvme_logpage_namespace_size_bytes")
	if !strings.Contains(size, `namespace="nvme0n1"`) {
		t.Errorf("namespace_size_bytes:\n%s", size)
	}
	if !strings.Contains(size, "5.12110190592e+11\n") {
		t.Errorf("namespace size does not match:\n%s", size)
	}
}

func TestNamespaceSectorMetric(t *testing.T) {
	e := testExporter(t)

	out := gatherText(t, e, "nvme_logpage_namespace_sector_bytes")
	if !strings.Contains(out, `namespace="nvme0n1"`) {
		t.Errorf("namespace_sector_bytes:\n%s", out)
	}
	if !strings.Contains(out, "} 512\n") {
		t.Errorf("namespace sector size does not match:\n%s", out)
	}
}

// The fixture uses md3, not md0, so a wrong read actually fails the test.
func TestNamespaceMDInfo(t *testing.T) {
	e := testExporter(t)

	out := gatherText(t, e, "nvme_logpage_namespace_md_info")
	if !strings.Contains(out, `md="md3"`) || !strings.Contains(out, `namespace="nvme0n1"`) {
		t.Errorf("nvme_logpage_namespace_md_info:\n%s", out)
	}
}

func TestNamespaceMDInfoNotEmittedWhenUnmapped(t *testing.T) {
	e := testExporter(t)

	out := gatherText(t, e, "nvme_logpage_namespace_md_info")
	if strings.Contains(out, `namespace="nvme0n2"`) {
		t.Errorf("nvme0n2 is not in the md mapping and must not get a namespace_md_info series:\n%s", out)
	}
	if !strings.Contains(out, `namespace="nvme0n1"`) {
		t.Errorf("nvme0n1 is in the md mapping and must still get a namespace_md_info series:\n%s", out)
	}
}

func TestControllerStateEmittedEvenWhenDeviceUnreadable(t *testing.T) {
	src := &stateSource{
		state: "resetting",
		err:   nvme.ErrNoCapability,
	}
	e := New(src, discardLogger())

	got := gatherText(t, e, "nvme_logpage_controller_state")
	if !strings.Contains(got, `device="nvme0",serial="SCRUBBED",state="resetting"} 1`) {
		t.Errorf("controller_state missing or wrong while the device is unreadable:\n%s", got)
	}
}

// Empty state: an older kernel, or a dump captured before the field existed.
func TestControllerStateOmittedWhenUnknown(t *testing.T) {
	src := &stateSource{state: ""}
	e := New(src, discardLogger())

	if got := gatherText(t, e, "nvme_logpage_controller_state"); got != "" {
		t.Errorf("controller_state emitted with no state known:\n%s", got)
	}
}

type stateSource struct {
	state string
	err   error
}

func (s *stateSource) Controllers() ([]nvme.Controller, error) {
	return []nvme.Controller{{Name: "nvme0", Serial: "SCRUBBED", State: s.state}}, nil
}
func (s *stateSource) Namespaces(string) ([]nvme.Namespace, error) { return nil, nil }
func (s *stateSource) MDMembership() (map[string]string, error)    { return map[string]string{}, nil }

func (s *stateSource) LogPage(context.Context, string, uint8, int) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return nil, nvme.ErrPageUnsupported
}

func (s *stateSource) Identify(context.Context, string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return nil, nvme.ErrPageUnsupported
}

func TestControllerLiveBoolean(t *testing.T) {
	cases := []struct {
		state string
		want  string
	}{
		{"live", `device="nvme0",serial="SCRUBBED"} 1`},
		{"resetting", `device="nvme0",serial="SCRUBBED"} 0`},
		{"dead", `device="nvme0",serial="SCRUBBED"} 0`},
		{"some-future-kernel-state", `device="nvme0",serial="SCRUBBED"} 0`},
	}
	for _, c := range cases {
		t.Run(c.state, func(t *testing.T) {
			e := New(&stateSource{state: c.state}, discardLogger())
			got := gatherText(t, e, "nvme_logpage_controller_live")
			if !strings.Contains(got, c.want) {
				t.Errorf("state %q: want %q in:\n%s", c.state, c.want, got)
			}
		})
	}
}

func TestControllerLiveOmittedWhenStateUnknown(t *testing.T) {
	e := New(&stateSource{state: ""}, discardLogger())
	if got := gatherText(t, e, "nvme_logpage_controller_live"); got != "" {
		t.Errorf("controller_live emitted with no state known:\n%s", got)
	}
}
