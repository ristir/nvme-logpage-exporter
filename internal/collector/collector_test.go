package collector

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
)

type toggleSource struct {
	nvme.Source
	fail bool
}

func (s *toggleSource) LogPage(ctx context.Context, controller string, id uint8, size int) ([]byte, error) {
	if s.fail {
		return nil, nvme.ErrNoCapability
	}
	return s.Source.LogPage(ctx, controller, id, size)
}

func drainCollect(e *Exporter) {
	ch := make(chan prometheus.Metric, 256)
	e.Collect(ch)
	close(ch)
	for range ch { //nolint:revive // draining is the point
	}
}

func TestLogDedupResetsAfterRecovery(t *testing.T) {
	good, err := nvme.NewReplay("../../testdata/dumps/synthetic-samsung")
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	src := &toggleSource{Source: good, fail: true}
	e := New(src, logger)

	const marker = "device polling failed"

	drainCollect(e) // first failure: logged
	drainCollect(e) // same failure again: must be deduped

	if got := strings.Count(logBuf.String(), marker); got != 1 {
		t.Fatalf("after two consecutive failures, expected exactly 1 log line, got %d:\n%s", got, logBuf.String())
	}

	src.fail = false
	drainCollect(e) // recovers: must clear the dedup entry

	src.fail = true
	drainCollect(e) // fails again: must be logged, not swallowed

	if got := strings.Count(logBuf.String(), marker); got != 2 {
		t.Errorf("after a recovery and a fresh failure, expected 2 log lines total, got %d:\n%s", got, logBuf.String())
	}
}
