package collector

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

func gatherText(t *testing.T, c prometheus.Collector, name string) string {
	t.Helper()

	reg := prometheus.NewPedanticRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	var sb strings.Builder
	enc := expfmt.NewEncoder(&sb, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		if err := enc.Encode(mf); err != nil {
			t.Fatalf("Encode: %v", err)
		}
	}
	return sb.String()
}

func collectOne(t *testing.T, c prometheus.Collector, name string) prometheus.Collector {
	t.Helper()

	ch := make(chan prometheus.Metric, 1024)
	c.Collect(ch)
	close(ch)

	var found prometheus.Metric
	for m := range ch {
		if strings.Contains(m.Desc().String(), `fqName: "`+name+`"`) {
			if found != nil {
				t.Fatalf("metric %s appears more than once", name)
			}
			found = m
		}
	}
	if found == nil {
		t.Fatalf("metric %s not found", name)
	}
	return singleMetricCollector{found}
}

type singleMetricCollector struct{ m prometheus.Metric }

func (s singleMetricCollector) Describe(ch chan<- *prometheus.Desc) { ch <- s.m.Desc() }
func (s singleMetricCollector) Collect(ch chan<- prometheus.Metric) { ch <- s.m }
