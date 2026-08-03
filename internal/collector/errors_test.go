package collector

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"device open", fmt.Errorf("wrap: %w", nvme.ErrDeviceOpen), "open"},
		{"missing capability", fmt.Errorf("wrap: %w", nvme.ErrNoCapability), "capability"},
		{"context deadline", context.DeadlineExceeded, "timeout"},
		{"parse error", errParse, "parse"},
		{"everything else", errors.New("something else"), "ioctl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyError(tt.err); got != tt.want {
				t.Errorf("classifyError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestUnsupportedIsNotAnError(t *testing.T) {
	if isDeviceError(fmt.Errorf("wrap: %w", nvme.ErrPageUnsupported)) {
		t.Fatal("ErrPageUnsupported was wrongly counted as a device failure")
	}
}
