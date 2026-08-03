package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewRegistryExposesBuildInfo(t *testing.T) {
	reg := NewRegistry()

	got, err := testutil.GatherAndCount(reg, "nvme_logpage_build_info")
	if err != nil {
		t.Fatalf("GatherAndCount: %v", err)
	}
	if got != 1 {
		t.Fatalf("nvme_logpage_build_info: got %d series, want 1", got)
	}
}

func TestNamespaceIsNvmeLog(t *testing.T) {
	if Namespace != "nvme_logpage" {
		t.Fatalf("Namespace = %q, want %q", Namespace, "nvme_logpage")
	}
}
