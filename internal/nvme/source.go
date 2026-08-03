package nvme

import (
	"context"
	"errors"
)

// ErrPageUnsupported is routine, not a failure: see nvme_logpage_supported.
var ErrPageUnsupported = errors.New("log page not supported by controller")

// Source supplies raw device data; splitting it here is what makes everything
// above testable without a drive.
type Source interface {
	Controllers() ([]Controller, error)
	Namespaces(controller string) ([]Namespace, error)

	// LogPage returns ErrPageUnsupported if the controller lacks the page.
	LogPage(ctx context.Context, controller string, pageID uint8, size int) ([]byte, error)

	// Identify returns the 4096-byte Identify Controller Data Structure.
	Identify(ctx context.Context, controller string) ([]byte, error)

	// No arrays is not an error.
	MDMembership() (map[string]string, error)
}
