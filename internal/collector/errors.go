package collector

import (
	"context"
	"errors"

	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
)

var errParse = errors.New("failed to parse page")

// Split by failure site: the access conditions have different fixes.
func classifyError(err error) string {
	switch {
	case errors.Is(err, nvme.ErrDeviceOpen):
		return "open"
	case errors.Is(err, nvme.ErrNoCapability):
		return "capability"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	case errors.Is(err, errParse):
		return "parse"
	default:
		return "ioctl"
	}
}

func isDeviceError(err error) bool {
	if err == nil {
		return false
	}
	return !errors.Is(err, nvme.ErrPageUnsupported)
}
