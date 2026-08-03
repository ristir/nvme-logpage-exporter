package collector

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

func TestCollectOCPForeignGUIDIsNotAnError(t *testing.T) {
	src, err := nvme.NewReplay("../../testdata/dumps/intel-p4510")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	e := New(src, discardLogger())

	avail := gatherText(t, e, "nvme_logpage_supported")
	if !strings.Contains(avail, `device="nvme0",page="0xc0",serial="SCRUBBED"} 0`) {
		t.Errorf("page 0xc0 not reported unavailable; got:\n%s", avail)
	}

	if got := gatherText(t, e, "nvme_logpage_media_written_bytes_total"); got != "" {
		t.Errorf("media_written_bytes_total emitted for a non-OCP drive:\n%s", got)
	}
	if got := gatherText(t, e, "nvme_logpage_ocp_info"); got != "" {
		t.Errorf("ocp_info emitted for a non-OCP drive:\n%s", got)
	}

	success := gatherText(t, e, "nvme_logpage_scrape_success")
	if !strings.Contains(success, `device="nvme0",serial="SCRUBBED"} 1`) {
		t.Errorf("scrape marked unsuccessful for a drive that simply lacks OCP:\n%s", success)
	}
}

// The KIOXIA fixture reports bad-system-NAND as all ones.
func TestCollectOCPRealPage(t *testing.T) {
	src, err := nvme.NewReplay("../../testdata/dumps/kioxia-kcd8")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	e := New(src, discardLogger())

	avail := gatherText(t, e, "nvme_logpage_supported")
	if !strings.Contains(avail, `device="nvme0",page="0xc0",serial="SCRUBBED"} 1`) {
		t.Errorf("page 0xc0 not reported available:\n%s", avail)
	}

	for _, name := range []string{
		"nvme_logpage_media_written_bytes_total",
		"nvme_logpage_media_read_bytes_total",
	} {
		if gatherText(t, e, name) == "" {
			t.Errorf("%s missing for a drive with a real OCP page", name)
		}
	}

	info := gatherText(t, e, "nvme_logpage_ocp_info")
	if !strings.Contains(info, `device="nvme0",serial="SCRUBBED",version="3"} 1`) {
		t.Errorf("ocp_info wrong or missing:\n%s", info)
	}

	bad := gatherText(t, e, "nvme_logpage_bad_nand_blocks_total")
	if strings.Contains(bad, `area="system"`) {
		t.Errorf(`area="system" emitted, but the fixture reports that field as all ones:\n%s`, bad)
	}
	if !strings.Contains(bad, `area="user"`) {
		t.Errorf(`area="user" missing, but the fixture implements that field:\n%s`, bad)
	}

	used := gatherText(t, e, "nvme_logpage_namespace_used_bytes")
	if !strings.Contains(used, `device="nvme0",namespace="nvme0n1",serial="SCRUBBED"}`) {
		t.Errorf("namespace_used_bytes missing the namespace label:\n%s", used)
	}
}

type noNamespaceSource struct{ nvme.Source }

func (noNamespaceSource) Namespaces(string) ([]nvme.Namespace, error) {
	return nil, nil
}

func TestCollectOCPNoNamespaceSkipsNamespaceUsed(t *testing.T) {
	inner, err := nvme.NewReplay("../../testdata/dumps/kioxia-kcd8")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	e := New(noNamespaceSource{Source: inner}, discardLogger())

	if got := gatherText(t, e, "nvme_logpage_namespace_used_bytes"); got != "" {
		t.Errorf("namespace_used_bytes emitted despite no enumerable namespace:\n%s", got)
	}

	for _, name := range []string{
		"nvme_logpage_media_written_bytes_total",
		"nvme_logpage_media_read_bytes_total",
		"nvme_logpage_ocp_info",
	} {
		if gatherText(t, e, name) == "" {
			t.Errorf("%s missing; only namespace_used_bytes should be skipped when namespaces don't enumerate", name)
		}
	}
}

type fixedNamespaceSource struct {
	nvme.Source
	names []string
}

func (s fixedNamespaceSource) Namespaces(ctrl string) ([]nvme.Namespace, error) {
	out := make([]nvme.Namespace, 0, len(s.names))
	for _, n := range s.names {
		out = append(out, nvme.Namespace{Name: n, Controller: ctrl})
	}
	return out, nil
}

func TestCollectOCPNamespaceUsedLabelsNamespaceOne(t *testing.T) {
	for _, tc := range []struct {
		name  string
		names []string
		want  string
	}{
		{"n1 behind n2 in directory order", []string{"nvme0n2", "nvme0n1"}, `namespace="nvme0n1"`},
		{"no namespace 1 at all", []string{"nvme0n2"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inner, err := nvme.NewReplay("../../testdata/dumps/kioxia-kcd8")
			if err != nil {
				t.Fatalf("NewReplay: %v", err)
			}
			e := New(fixedNamespaceSource{Source: inner, names: tc.names}, discardLogger())

			got := gatherText(t, e, "nvme_logpage_namespace_used_bytes")
			if tc.want == "" {
				if got != "" {
					t.Errorf("namespace_used_bytes emitted without a namespace 1:\n%s", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("namespace_used_bytes not labelled %s:\n%s", tc.want, got)
			}
		})
	}
}

// Live drives report 162 and 231 for a field the spec calls a percentage.
func TestCollectOCPCapacitorHealthIsUnscaled(t *testing.T) {
	src, err := nvme.NewReplay("../../testdata/dumps/samsung-datacenter")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	e := New(src, discardLogger())

	if got := gatherText(t, e, "nvme_logpage_capacitor_health_ratio"); got != "" {
		t.Errorf("capacitor health emitted as a ratio; the field is not a percentage:\n%s", got)
	}

	const want = 160
	if got := testutil.ToFloat64(collectOne(t, e, "nvme_logpage_capacitor_health")); got != want {
		t.Errorf("nvme_logpage_capacitor_health = %v, want %v (unscaled raw value)", got, want)
	}
}

type countingSource struct {
	nvme.Source

	page uint8

	mu    sync.Mutex
	calls int
}

func (s *countingSource) LogPage(ctx context.Context, controller string, pageID uint8, size int) ([]byte, error) {
	if pageID == s.page {
		s.mu.Lock()
		s.calls++
		s.mu.Unlock()
	}
	return s.Source.LogPage(ctx, controller, pageID, size)
}

func (s *countingSource) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestCollectOCPProbeCaching(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		want    int
		why     string
	}{
		{"unsupported page is probed once", "micron-3500", 2,
			"two controllers with no 0xC0 at all: one probe each, ever"},
		{"foreign GUID is probed once", "intel-p4510", 2,
			"two controllers answering 0xC0 with non-OCP data: one probe each, ever"},
		{"real page is read every scrape", "kioxia-kcd8", 6,
			"two controllers with a real OCP page: the data itself must be re-read"},
	}

	const scrapes = 3

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner, err := nvme.NewReplay("../../testdata/dumps/" + tc.fixture)
			if err != nil {
				t.Fatalf("NewReplay: %v", err)
			}
			src := &countingSource{Source: inner, page: logpage.IDOCPSmart}
			e := New(src, discardLogger())

			for i := 0; i < scrapes; i++ {
				testutil.CollectAndCount(e)
			}

			if got := src.count(); got != tc.want {
				t.Errorf("0xC0 read %d times across %d scrapes, want %d — %s",
					got, scrapes, tc.want, tc.why)
			}
		})
	}
}

// synthetic-ocp carries 0x02 and 0xC0 only, so 0x01 and 0x03 are refused.
func TestUnsupportedMandatoryPageProbedOnce(t *testing.T) {
	const scrapes = 3

	for _, tc := range []struct {
		label string
		id    uint8
	}{
		{"0x01", logpage.IDErrorInfo},
		{"0x03", logpage.IDFirmwareSlot},
	} {
		t.Run(tc.label, func(t *testing.T) {
			inner, err := nvme.NewReplay("../../testdata/dumps/synthetic-ocp")
			if err != nil {
				t.Fatalf("NewReplay: %v", err)
			}
			src := &countingSource{Source: inner, page: tc.id}
			e := New(src, discardLogger())

			for i := 0; i < scrapes; i++ {
				testutil.CollectAndCount(e)
			}

			if got := src.count(); got != 1 {
				t.Errorf("page %s read %d times across %d scrapes, want 1", tc.label, got, scrapes)
			}

			last := gatherText(t, e, "nvme_logpage_supported")
			want := `device="nvme0",page="` + tc.label + `",serial="SCRUBBED"} 0`
			if !strings.Contains(last, want) {
				t.Errorf("page %s gauge lost once the refusal was cached:\n%s", tc.label, last)
			}
		})
	}
}

func TestCollectOCPUnavailableGaugeSurvivesCaching(t *testing.T) {
	src, err := nvme.NewReplay("../../testdata/dumps/micron-3500")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	e := New(src, discardLogger())

	gatherText(t, e, "nvme_logpage_supported") // first scrape, populates the cache
	second := gatherText(t, e, "nvme_logpage_supported")

	if !strings.Contains(second, `device="nvme0",page="0xc0",serial="SCRUBBED"} 0`) {
		t.Errorf("page 0xc0 gauge missing on the second scrape:\n%s", second)
	}
}

func TestCollectFirmwareSlots(t *testing.T) {
	src, err := nvme.NewReplay("../../testdata/dumps/dell-p4510")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	e := New(src, discardLogger())

	info := gatherText(t, e, "nvme_logpage_firmware_slot_info")
	for _, want := range []string{
		`revision="VDV1DP23",serial="SCRUBBED",slot="1"`,
		`revision="VDV1DP25",serial="SCRUBBED",slot="2"`,
	} {
		if !strings.Contains(info, want) {
			t.Errorf("missing %q in:\n%s", want, info)
		}
	}
	if strings.Contains(info, `revision=""`) {
		t.Errorf("empty slot emitted:\n%s", info)
	}
	if strings.Contains(info, `slot="3"`) {
		t.Errorf("unpopulated slot 3 emitted:\n%s", info)
	}

	active := gatherText(t, e, "nvme_logpage_firmware_active_slot")
	if !strings.Contains(active, `device="nvme0",serial="SCRUBBED"} 2`) {
		t.Errorf("active slot wrong or missing:\n%s", active)
	}

	if got := gatherText(t, e, "nvme_logpage_firmware_next_slot"); got != "" {
		t.Errorf("next-slot series emitted with no activation pending:\n%s", got)
	}
}

// Thirteen of fourteen fleet dumps have a completely empty error log.
func TestCollectErrorInfoEmptyLogEmitsNoSeries(t *testing.T) {
	src, err := nvme.NewReplay("../../testdata/dumps/micron-3500")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	e := New(src, discardLogger())

	if got := gatherText(t, e, "nvme_logpage_error_log_retained_entries"); got != "" {
		t.Errorf("retained-entries series emitted for an empty log:\n%s", got)
	}

	avail := gatherText(t, e, "nvme_logpage_supported")
	if !strings.Contains(avail, `device="nvme0",page="0x01",serial="SCRUBBED"} 1`) {
		t.Errorf("page 0x01 not reported available:\n%s", avail)
	}
}

// The one non-empty error log on the fleet: count 178, raw status 0x4004.
func TestCollectErrorInfoWithEntriesAggregatesByStatus(t *testing.T) {
	src, err := nvme.NewReplay("../../testdata/dumps/samsung-datacenter")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	e := New(src, discardLogger())

	out := gatherText(t, e, "nvme_logpage_error_log_retained_entries")
	want := `device="nvme1",serial="SCRUBBED",status_code="0x02",status_code_type="0"} 1`
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}

	if got := testutil.ToFloat64(collectOne(t, e, "nvme_logpage_error_log_retained_entries")); got != 1 {
		t.Errorf("nvme_logpage_error_log_retained_entries = %v, want 1", got)
	}
}
