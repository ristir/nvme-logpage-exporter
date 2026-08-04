package collector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
)

type brokenSource struct{ ok nvme.Source }

func (b brokenSource) Controllers() ([]nvme.Controller, error) {
	c, err := b.ok.Controllers()
	if err != nil {
		return nil, err
	}
	return append(c, nvme.Controller{Name: "nvme1", DevPath: "/dev/nvme1", Serial: "BROKEN"}), nil
}

func (b brokenSource) Namespaces(controller string) ([]nvme.Namespace, error) {
	if controller == "nvme1" {
		return nil, nil
	}
	return b.ok.Namespaces(controller)
}

func (b brokenSource) LogPage(ctx context.Context, controller string, id uint8, size int) ([]byte, error) {
	if controller == "nvme1" {
		return nil, nvme.ErrNoCapability
	}
	return b.ok.LogPage(ctx, controller, id, size)
}

func (b brokenSource) Identify(ctx context.Context, controller string) ([]byte, error) {
	if controller == "nvme1" {
		return nil, nvme.ErrNoCapability
	}
	return b.ok.Identify(ctx, controller)
}

func (b brokenSource) MDMembership() (map[string]string, error) {
	return map[string]string{}, nil
}

func TestBrokenDeviceDoesNotKillScrape(t *testing.T) {
	good, err := nvme.NewReplay("../../testdata/dumps/synthetic-samsung")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	h := Handler(brokenSource{ok: good}, discardLogger(), 5*time.Second)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	if !strings.Contains(body, `nvme_logpage_scrape_success{device="nvme0",serial="SYNTHETIC0000001"} 1`) {
		t.Errorf("the healthy device should have scrape_success 1")
	}
	if !strings.Contains(body, `nvme_logpage_scrape_success{device="nvme1",serial="BROKEN"} 0`) {
		t.Errorf("the broken device should have scrape_success 0")
	}
	if !strings.Contains(body, `reason="capability"`) {
		t.Errorf("the failure reason should show up in errors_total:\n%s", body)
	}
	if !strings.Contains(body, "nvme_logpage_temperature_celsius") {
		t.Errorf("the healthy device's metrics disappeared:\n%s", body)
	}
	if !strings.Contains(body, "nvme_logpage_devices_discovered 2") {
		t.Errorf("devices_discovered should be 2:\n%s", body)
	}
}

const collectorsPerBrokenDeviceScrape = 5

func TestDeviceErrorCounterPersistsAcrossScrapes(t *testing.T) {
	good, err := nvme.NewReplay("../../testdata/dumps/synthetic-samsung")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	h := Handler(brokenSource{ok: good}, discardLogger(), 5*time.Second)

	bodies := make([]string, 2)
	for i := range bodies {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("scrape %d: code %d, want 200", i+1, rec.Code)
		}
		bodies[i] = rec.Body.String()
	}

	const line = `nvme_logpage_errors_total{device="nvme1",reason="capability",serial="BROKEN"} %d`
	if want := fmt.Sprintf(line, collectorsPerBrokenDeviceScrape); !strings.Contains(bodies[0], want) {
		t.Errorf("after the first scrape:\nwant substring: %s\ngot body:\n%s", want, bodies[0])
	}
	if want := fmt.Sprintf(line, 2*collectorsPerBrokenDeviceScrape); !strings.Contains(bodies[1], want) {
		t.Errorf("after the second scrape, the counter must have kept accumulating, not reset:\nwant substring: %s\ngot body:\n%s", want, bodies[1])
	}
}

type deadSource struct{ brokenSource }

func (deadSource) Controllers() ([]nvme.Controller, error) {
	return nil, errors.New("sysfs unavailable")
}

func TestEnumerationFailureReturns500(t *testing.T) {
	h := Handler(deadSource{}, discardLogger(), time.Second)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code %d, want 500", rec.Code)
	}
}

type countingControllersSource struct {
	nvme.Source
	mu    sync.Mutex
	calls int
}

func (s *countingControllersSource) Controllers() ([]nvme.Controller, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.Source.Controllers()
}

func TestControllersCalledExactlyOncePerRequest(t *testing.T) {
	good, err := nvme.NewReplay("../../testdata/dumps/synthetic-samsung")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	src := &countingControllersSource{Source: good}

	h := Handler(src, discardLogger(), time.Second)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code %d, want 200", rec.Code)
	}

	src.mu.Lock()
	calls := src.calls
	src.mu.Unlock()
	if calls != 1 {
		t.Errorf("Controllers() called %d times per request, want exactly 1", calls)
	}
}

func TestScrapeDeadline(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   time.Duration
		wantOK bool
	}{
		{"absent header", "", 0, false},
		{"unparseable", "soon please", 0, false},
		{"negative", "-1", 0, false},
		{"zero", "0", 0, false},
		{"valid", "2.5", 2500 * time.Millisecond, true},
		{"NaN", "NaN", 0, false},
		{"negative infinity", "-Inf", 0, false},
		{"positive infinity", "Inf", maxScrapeTimeout, true},
		{"beyond int64 nanoseconds", "1e18", maxScrapeTimeout, true},
		{"above the ceiling", "7200", maxScrapeTimeout, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tt.header != "" {
				req.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", tt.header)
			}
			got, ok := scrapeDeadline(req)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("scrapeDeadline(%q) = (%v, %v), want (%v, %v)", tt.header, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

type deadlineRecordingSource struct {
	nvme.Source

	mu       sync.Mutex
	deadline time.Time
	ok       bool
}

func (s *deadlineRecordingSource) LogPage(ctx context.Context, controller string, id uint8, size int) ([]byte, error) {
	deadline, ok := ctx.Deadline()
	s.mu.Lock()
	s.deadline, s.ok = deadline, ok
	s.mu.Unlock()
	return s.Source.LogPage(ctx, controller, id, size)
}

func TestScrapeTimeoutHeaderRespected(t *testing.T) {
	good, err := nvme.NewReplay("../../testdata/dumps/synthetic-samsung")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	rec := &deadlineRecordingSource{Source: good}

	h := Handler(rec, discardLogger(), time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", "2.5")

	start := time.Now()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "nvme_logpage_scrape_duration_seconds") {
		t.Errorf("duration metric missing:\n%s", w.Body.String())
	}

	rec.mu.Lock()
	deadline, ok := rec.deadline, rec.ok
	rec.mu.Unlock()
	if !ok {
		t.Fatal("LogPage was never called with a deadline-bearing context")
	}
	observed := deadline.Sub(start)
	const want = 2500 * time.Millisecond
	const slack = 500 * time.Millisecond
	if observed < want-slack || observed > want+slack {
		t.Errorf("observed deadline %v from request start, want ~%v (the header value, not the 1h deviceTimeout)", observed, want)
	}
}

func TestLogPageAvailableReported(t *testing.T) {
	good, err := nvme.NewReplay("../../testdata/dumps/synthetic-samsung")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	h := Handler(good, discardLogger(), time.Second)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := rec.Body.String()
	if !strings.Contains(body, `nvme_logpage_supported{device="nvme0",page="0x02",serial="SYNTHETIC0000001"} 1`) {
		t.Errorf("page 0x02 should be reported as available:\n%s", body)
	}
}

type unsupportedPageSource struct{ nvme.Source }

func (unsupportedPageSource) LogPage(context.Context, string, uint8, int) ([]byte, error) {
	return nil, nvme.ErrPageUnsupported
}

func TestUnsupportedPageIsNotCountedAsError(t *testing.T) {
	good, err := nvme.NewReplay("../../testdata/dumps/synthetic-samsung")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	h := Handler(unsupportedPageSource{Source: good}, discardLogger(), time.Second)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	if !strings.Contains(body, `nvme_logpage_supported{device="nvme0",page="0x02",serial="SYNTHETIC0000001"} 0`) {
		t.Errorf("unsupported page should be reported unavailable:\n%s", body)
	}
	if strings.Contains(body, "nvme_logpage_errors_total") {
		t.Errorf("an unsupported page must not produce any errors_total series:\n%s", body)
	}
	if !strings.Contains(body, `nvme_logpage_scrape_success{device="nvme0",serial="SYNTHETIC0000001"} 1`) {
		t.Errorf("an unsupported page alone must not fail the scrape:\n%s", body)
	}
}
