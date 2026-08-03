package collector

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/ristir/nvme-logpage-exporter/internal/metrics"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
)

// Handler serves device metrics. The registry is per request because
// prometheus.Collector takes no context and the deadline must reach Collect.
func Handler(src nvme.Source, logger *slog.Logger, deviceTimeout time.Duration) http.Handler {
	shared := New(src, logger)
	shared.deviceTimeout = deviceTimeout

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 500, not an empty 200, which is indistinguishable from "no NVMe here".
		ctrls, err := src.Controllers()
		if err != nil {
			logger.Error("failed to enumerate NVMe devices", "err", err)
			http.Error(w, "failed to enumerate NVMe devices: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if ctrls == nil {
			ctrls = []nvme.Controller{}
		}

		ctx := r.Context()
		if d, ok := scrapeDeadline(r); ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}

		perRequest := *shared
		perRequest.ctx = ctx
		perRequest.ctrls = ctrls

		reg := metrics.NewRegistry()
		reg.MustRegister(&perRequest)

		promhttp.HandlerFor(reg, promhttp.HandlerOpts{
			ErrorHandling: promhttp.ContinueOnError,
			// Without this a Gather failure yields a silently truncated 200.
			ErrorLog: slogErrorLog{logger: logger},
		}).ServeHTTP(w, r)
	})
}

// Adapts *slog.Logger to promhttp.Logger.
type slogErrorLog struct{ logger *slog.Logger }

func (l slogErrorLog) Println(v ...any) {
	l.logger.Error(fmt.Sprint(v...))
}

func scrapeDeadline(r *http.Request) (time.Duration, bool) {
	v := r.Header.Get("X-Prometheus-Scrape-Timeout-Seconds")
	if v == "" {
		return 0, false
	}
	secs, err := strconv.ParseFloat(v, 64)
	if err != nil || secs <= 0 {
		return 0, false
	}
	return time.Duration(secs * float64(time.Second)), true
}
