package collector

import (
	"encoding/binary"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

func buildRawSmart(compositeKelvin uint16, sensorsKelvin map[int]uint16) []byte {
	b := make([]byte, logpage.SmartLogSize)
	binary.LittleEndian.PutUint16(b[1:3], compositeKelvin)
	for idx, k := range sensorsKelvin {
		off := 200 + (idx-1)*2
		binary.LittleEndian.PutUint16(b[off:off+2], k)
	}
	return b
}

func testExporter(t *testing.T) *Exporter {
	t.Helper()
	src, err := nvme.NewReplay("../../testdata/dumps/synthetic-samsung")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	return New(src, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestSmartMetricsMatchGolden(t *testing.T) {
	e := testExporter(t)

	want, err := os.Open("testdata/expected-synthetic-samsung.prom")
	if err != nil {
		t.Fatalf("golden: %v", err)
	}
	defer func() { _ = want.Close() }()

	names := []string{
		"nvme_logpage_temperature_celsius",
		"nvme_logpage_composite_temperature_celsius",
		"nvme_logpage_available_spare_ratio",
		"nvme_logpage_available_spare_threshold_ratio",
		"nvme_logpage_endurance_used_ratio",
		"nvme_logpage_read_bytes_total",
		"nvme_logpage_written_bytes_total",
		"nvme_logpage_power_on_seconds_total",
		"nvme_logpage_controller_busy_seconds_total",
		"nvme_logpage_warning_temperature_seconds_total",
		"nvme_logpage_critical_temperature_seconds_total",
		"nvme_logpage_critical_warning_flag",
	}
	if err := testutil.CollectAndCompare(e, want, names...); err != nil {
		t.Errorf("mismatch with golden:\n%v", err)
	}
}

func TestSmartUnitConversions(t *testing.T) {
	e := testExporter(t)

	cases := []struct {
		metric string
		want   float64
	}{
		{"nvme_logpage_power_on_seconds_total", 1844 * 3600},
		{"nvme_logpage_controller_busy_seconds_total", 1448 * 60},
		{"nvme_logpage_warning_temperature_seconds_total", 7 * 60},
		{"nvme_logpage_critical_temperature_seconds_total", 3 * 60},
		{"nvme_logpage_read_bytes_total", 17122402 * 512000},
		{"nvme_logpage_written_bytes_total", 26169783 * 512000},
	}
	for _, c := range cases {
		got := testutil.ToFloat64(collectOne(t, e, c.metric))
		if got != c.want {
			t.Errorf("%s = %v, want %v", c.metric, got, c.want)
		}
	}
}

func TestCriticalWarningFlags(t *testing.T) {
	e := testExporter(t)

	out := gatherText(t, e, "nvme_logpage_critical_warning_flag")

	// expfmt sorts labels alphabetically, so serial trails every series here.
	const serial = `,serial="SYNTHETIC0000001"}`
	mustContain := []string{
		`flag="spare_below_threshold"` + serial + ` 1`,
		`flag="reliability_degraded"` + serial + ` 1`,
		`flag="temperature"` + serial + ` 0`,
		`flag="read_only"` + serial + ` 0`,
		`flag="volatile_backup_failed"` + serial + ` 0`,
		`flag="persistent_memory_ro"` + serial + ` 0`,
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("output is missing %q:\n%s", s, out)
		}
	}
	if strings.Contains(out, "reserved_") {
		t.Errorf("no reserved bits are set, but they are emitted:\n%s", out)
	}
}

func TestTemperatureSensorsOnlyPresent(t *testing.T) {
	e := testExporter(t)

	out := gatherText(t, e, "nvme_logpage_temperature_celsius")

	for _, s := range []string{`sensor="1"`, `sensor="2"`} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q:\n%s", s, out)
		}
	}
	if strings.Contains(out, `sensor="3"`) {
		t.Errorf("sensor 3 is absent from the dump, but is emitted:\n%s", out)
	}
	if strings.Contains(out, `sensor="composite"`) {
		t.Errorf("the composite reading must not appear as a sensor value in nvme_logpage_temperature_celsius:\n%s", out)
	}
}

func TestCompositeTemperatureIsSeparateMetric(t *testing.T) {
	e := testExporter(t)

	out := gatherText(t, e, "nvme_logpage_composite_temperature_celsius")

	if strings.Contains(out, "sensor=") {
		t.Errorf("nvme_logpage_composite_temperature_celsius must not carry a sensor label:\n%s", out)
	}
	if !strings.Contains(out, `serial="SYNTHETIC0000001"} 30.850000000000023`) {
		t.Errorf("composite temperature value:\n%s", out)
	}
}

// A zero composite reading is the spec's convention for "not implemented".
func TestCompositeTemperatureNotEmittedWhenZero(t *testing.T) {
	src := fakeSource{
		controller: nvme.Controller{Name: "nvme0", Serial: "SYNTHETIC0000097"},
		smartRaw:   buildRawSmart(0, map[int]uint16{1: 300}),
	}
	e := New(src, discardLogger())

	composite := gatherText(t, e, "nvme_logpage_composite_temperature_celsius")
	if composite != "" {
		t.Errorf("zero composite reading must not be emitted:\n%s", composite)
	}

	sensors := gatherText(t, e, "nvme_logpage_temperature_celsius")
	if !strings.Contains(sensors, `sensor="1"`) {
		t.Errorf("sensor 1 is present in the fixture and must still be emitted:\n%s", sensors)
	}
}

func TestMetricsPassPromlint(t *testing.T) {
	e := testExporter(t)

	problems, err := testutil.CollectAndLint(e)
	if err != nil {
		t.Fatalf("CollectAndLint: %v", err)
	}
	for _, p := range problems {
		t.Errorf("promlint: %s: %s", p.Metric, p.Text)
	}
}

func TestAllMetricsUseNamespacePrefix(t *testing.T) {
	e := testExporter(t)

	reg := prometheus.NewPedanticRegistry()
	if err := reg.Register(e); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	if len(mfs) == 0 {
		t.Fatal("no metrics collected; nothing to validate")
	}

	const prefix = "nvme_logpage_"
	for _, mf := range mfs {
		name := mf.GetName()
		if !strings.HasPrefix(name, prefix) {
			t.Errorf("metric %q does not use the required namespace prefix %q", name, prefix)
		}
	}
}
