package collector

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

// Answers only page 0x06, so no other collector can satisfy the assertions.
type selfTestSource struct {
	nvme.Source
	page []byte
}

func (s selfTestSource) LogPage(ctx context.Context, controller string, id uint8, size int) ([]byte, error) {
	if id == logpage.IDSelfTest {
		if s.page == nil {
			return nil, nvme.ErrPageUnsupported
		}
		return s.page, nil
	}
	return nil, nvme.ErrPageUnsupported
}

func (selfTestSource) Controllers() ([]nvme.Controller, error) {
	return []nvme.Controller{{Name: "nvme0", DevPath: "/dev/nvme0", Serial: "S1", State: "live"}}, nil
}

func (selfTestSource) Namespaces(string) ([]nvme.Namespace, error) { return nil, nil }

func (selfTestSource) Identify(context.Context, string) ([]byte, error) {
	return nil, nvme.ErrPageUnsupported
}

func (selfTestSource) MDMembership() (map[string]string, error) { return map[string]string{}, nil }

func selfTestPageBytes(inProgress, completion byte, entries ...[4]uint64) []byte {
	b := make([]byte, logpage.SelfTestSize)
	b[0] = inProgress
	b[1] = completion
	for i := 0; i < 20; i++ {
		b[4+i*28] = 0x0F
	}
	// entry = {status, segment, powerOnHours, unused}
	for i, e := range entries {
		o := 4 + i*28
		b[o] = byte(e[0])
		b[o+1] = byte(e[1])
		binary.LittleEndian.PutUint64(b[o+4:o+12], e[2])
	}
	return b
}

func scrapeSelfTest(t *testing.T, page []byte) string {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler(selfTestSource{page: page}, discardLogger(), 5*time.Second).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d, want 200", rec.Code)
	}
	return rec.Body.String()
}

func TestSelfTestGroupsResultsByOutcomeAndType(t *testing.T) {
	// Two short tests aborted by a controller reset, one short pass, one
	// extended run that failed a segment.
	body := scrapeSelfTest(t, selfTestPageBytes(0, 0,
		[4]uint64{0x12, 0, 0x2edc}, // short, aborted by reset
		[4]uint64{0x10, 0, 0x2eda}, // short, passed
		[4]uint64{0x12, 0, 0x218e}, // short, aborted by reset
		[4]uint64{0x27, 0, 0x1000}, // extended, failed segment
	))

	for _, want := range []string{
		`nvme_logpage_self_test_results{device="nvme0",result="aborted_by_reset",serial="S1",test="short"} 2`,
		`nvme_logpage_self_test_results{device="nvme0",result="passed",serial="S1",test="short"} 1`,
		`nvme_logpage_self_test_results{device="nvme0",result="failed_segment",serial="S1",test="extended"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s\n%s", want, body)
		}
	}
}

func TestSelfTestLastResultIsTheNewestEntry(t *testing.T) {
	body := scrapeSelfTest(t, selfTestPageBytes(0, 0,
		[4]uint64{0x15, 0, 100}, // newest: short, fatal error
		[4]uint64{0x10, 0, 90},  // older: short, passed
	))

	if !strings.Contains(body, `nvme_logpage_self_test_last_result{device="nvme0",serial="S1"} 5`) {
		t.Errorf("last_result should come from entry 0:\n%s", body)
	}
	// 100 hours in seconds.
	if !strings.Contains(body, `nvme_logpage_self_test_last_power_on_seconds{device="nvme0",serial="S1"} 360000`) {
		t.Errorf("last_power_on_seconds should convert hours to seconds:\n%s", body)
	}
}

func TestSelfTestEmptyLogEmitsNoResults(t *testing.T) {
	body := scrapeSelfTest(t, selfTestPageBytes(0, 0))

	if strings.Contains(body, "nvme_logpage_self_test_results{") {
		t.Errorf("an empty log must not produce result series:\n%s", body)
	}
	if strings.Contains(body, "nvme_logpage_self_test_last_result") {
		t.Errorf("an empty log has no last result:\n%s", body)
	}
	if !strings.Contains(body, `nvme_logpage_self_test_running{device="nvme0",serial="S1"} 0`) {
		t.Errorf("running should still be reported:\n%s", body)
	}
}

func TestSelfTestReportsProgressOnlyWhileRunning(t *testing.T) {
	idle := scrapeSelfTest(t, selfTestPageBytes(0, 0))
	if strings.Contains(idle, "nvme_logpage_self_test_completion_ratio") {
		t.Errorf("an idle drive must not report completion:\n%s", idle)
	}

	running := scrapeSelfTest(t, selfTestPageBytes(2, 65))
	if !strings.Contains(running, `nvme_logpage_self_test_running{device="nvme0",serial="S1"} 1`) {
		t.Errorf("running should be 1:\n%s", running)
	}
	if !strings.Contains(running, `nvme_logpage_self_test_completion_ratio{device="nvme0",serial="S1"} 0.65`) {
		t.Errorf("completion should be a 0..1 ratio:\n%s", running)
	}
}

func TestSelfTestUnsupportedPageIsNotAnError(t *testing.T) {
	body := scrapeSelfTest(t, nil)

	if !strings.Contains(body, `nvme_logpage_supported{device="nvme0",page="0x06",serial="S1"} 0`) {
		t.Errorf("an absent page should be reported as unsupported:\n%s", body)
	}
	if strings.Contains(body, `reason="ioctl"`) {
		t.Errorf("an absent page must not count as a device error:\n%s", body)
	}
}
