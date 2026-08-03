package collector

import (
	"context"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

type sizeRecordingSource struct {
	nvme.Source

	mu    sync.Mutex
	sizes []int
}

func (s *sizeRecordingSource) LogPage(ctx context.Context, controller string, id uint8, size int) ([]byte, error) {
	if id == logpage.IDErrorInfo {
		s.mu.Lock()
		s.sizes = append(s.sizes, size)
		s.mu.Unlock()
	}
	return s.Source.LogPage(ctx, controller, id, size)
}

func (s *sizeRecordingSource) recorded() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.sizes...)
}

type noIdentifySource struct{ nvme.Source }

func (noIdentifySource) Identify(context.Context, string) ([]byte, error) {
	return nil, nvme.ErrPageUnsupported
}

// The fixture reports ELPE 63, so the log holds 64 entries of 64 bytes.
func TestErrorLogReadCoversWholeLog(t *testing.T) {
	inner, err := nvme.NewReplay("../../testdata/dumps/samsung-errorlog-full")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	src := &sizeRecordingSource{Source: inner}
	testutil.CollectAndCount(New(src, discardLogger()))

	got := src.recorded()
	if len(got) == 0 {
		t.Fatal("page 0x01 was never read")
	}
	for _, size := range got {
		if size != 64*logpage.ErrorInfoEntrySize {
			t.Errorf("read %d bytes of page 0x01, want %d: a fixed read truncates a full log",
				size, 64*logpage.ErrorInfoEntrySize)
		}
	}
}

func TestErrorLogFallsBackWithoutIdentify(t *testing.T) {
	inner, err := nvme.NewReplay("../../testdata/dumps/samsung-errorlog-full")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}
	src := &sizeRecordingSource{Source: noIdentifySource{Source: inner}}
	testutil.CollectAndCount(New(src, discardLogger()))

	for _, size := range src.recorded() {
		if size != errorInfoFallbackSize {
			t.Errorf("read %d bytes without Identify, want the %d-byte fallback", size, errorInfoFallbackSize)
		}
	}
}
