// Package metrics holds the shared Prometheus registry and the metric prefix.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
)

// Namespace prefixes every metric; unique so several disk exporters on one
// host stay distinguishable after SD strips the port.
const Namespace = "nvme_logpage"

// NewRegistry registers the Go, process and version collectors explicitly.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		versioncollector.NewCollector(Namespace),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}
