// Command nvme_logpage_exporter exports NVMe log pages read over ioctl.
package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/common/promslog"
	promslogflag "github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/ristir/nvme-logpage-exporter/internal/collector"
	"github.com/ristir/nvme-logpage-exporter/internal/dump"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
)

// A stalled client must not hold a connection on a CAP_SYS_ADMIN process.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
)

func main() {
	var (
		webConfig   = webflag.AddFlags(kingpin.CommandLine, ":9683")
		metricsPath = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").
				Default("/metrics").String()
		sourceSpec = kingpin.Flag("nvme.source",
			"Data source: auto (ioctl) or dir:<path> to replay dumps.").
			Default("auto").String()
		deviceTimeout = kingpin.Flag("nvme.timeout",
			"Timeout for polling a single device.").Default("5s").Duration()
		promslogConfig = &promslog.Config{}
	)

	kingpin.Command("serve", "Run the exporter.").Default()
	dumpCmd := kingpin.Command("dump", "Capture log page dumps for tests and bug reports.")
	dumpOut := dumpCmd.Flag("out", "Directory to write the dump into.").Required().String()
	dumpKeepSerial := dumpCmd.Flag("keep-serial",
		"Do not scrub serial numbers. They are scrubbed by default because "+
			"dumps end up attached to public issues.").
		Bool()

	promslogflag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.CommandLine.UsageWriter(os.Stdout)
	kingpin.Version(version.Print("nvme_logpage_exporter"))
	kingpin.HelpFlag.Short('h')
	cmd := kingpin.Parse()

	logger := promslog.New(promslogConfig)
	logger.Info("starting nvme_logpage_exporter", "version", version.Info(), "build_context", version.BuildContext())

	src, err := nvme.Open(*sourceSpec)
	if err != nil {
		logger.Error("failed to open data source", "source", *sourceSpec, "err", err)
		os.Exit(1)
	}

	switch cmd {
	case "dump":
		if err := dump.Run(context.Background(), src, *dumpOut, *dumpKeepSerial, logger); err != nil {
			logger.Error("failed to capture dumps", "err", err)
			os.Exit(1)
		}
		logger.Info("dumps captured", "out", *dumpOut, "serial_scrubbed", !*dumpKeepSerial)
		return
	}

	mux := http.NewServeMux()
	mux.Handle(*metricsPath, collector.Handler(src, logger, *deviceTimeout))

	landing, err := web.NewLandingPage(web.LandingConfig{
		Name:        "NVMe logpage exporter",
		Description: "NVMe drive metrics, read directly via ioctl.",
		Version:     version.Info(),
		Links: []web.LandingLinks{
			{Address: *metricsPath, Text: "Metrics"},
		},
	})
	if err != nil {
		logger.Error("failed to build landing page", "err", err)
		os.Exit(1)
	}
	mux.Handle("/", landing)

	// Own mux: importing net/http/pprof would expose /debug/pprof by side effect.
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
	}
	if err := web.ListenAndServe(srv, webConfig, logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
