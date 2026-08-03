//go:build !linux

package nvme

import "errors"

// NewIoctl is Linux-only; the stub lets parser tests build elsewhere.
func NewIoctl(SysFS) (Source, error) {
	return nil, errors.New("the ioctl production source is only available on Linux; use --nvme.source=dir:<directory>")
}
