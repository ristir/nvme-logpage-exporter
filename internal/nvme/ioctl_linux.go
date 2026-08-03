//go:build linux

package nvme

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Reads log pages through NVME_IOCTL_ADMIN_CMD. No external processes.
type ioctlSource struct {
	sys SysFS
}

// NewIoctl returns the production source. Linux only; see ioctl_stub.go.
func NewIoctl(sys SysFS) (Source, error) {
	return &ioctlSource{sys: sys}, nil
}

func (i *ioctlSource) Controllers() ([]Controller, error) { return i.sys.Controllers() }

func (i *ioctlSource) Namespaces(controller string) ([]Namespace, error) {
	return i.sys.Namespaces(controller)
}

func (i *ioctlSource) LogPage(ctx context.Context, controller string, pageID uint8, size int) ([]byte, error) {
	if _, err := logPageSize(size); err != nil {
		return nil, err
	}
	return i.admin(ctx, controller, passthruCmd{
		opcode: opGetLogPage,
		nsid:   nsidAll,
		cdw10:  logPageCDW10(pageID, size),
	}, size)
}

func (i *ioctlSource) Identify(ctx context.Context, controller string) ([]byte, error) {
	return i.admin(ctx, controller, passthruCmd{
		opcode: opIdentify,
		nsid:   0,
		cdw10:  cnsController,
	}, IdentifySize)
}

func (i *ioctlSource) MDMembership() (map[string]string, error) { return i.sys.MDMembership() }

// The deadline goes to the kernel: an ioctl on a wedged controller sits in
// uninterruptible sleep where no Go timer reaches it.
func (i *ioctlSource) admin(ctx context.Context, controller string, cmd passthruCmd, size int) ([]byte, error) {
	path := filepath.Join(i.sys.DevRoot, controller)

	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, wrapDeviceOpen(path, err)
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, size)
	cmd.addr = uint64(uintptr(unsafe.Pointer(&buf[0])))
	cmd.dataLen = uint32(size)
	cmd.timeoutMs = timeoutMillis(ctx)

	ret, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		f.Fd(),
		uintptr(nvmeIoctlAdminCmd),
		uintptr(unsafe.Pointer(&cmd)),
	)
	// The GC cannot see the stashed pointer as a reference.
	runtime.KeepAlive(buf)

	switch {
	case errors.Is(errno, unix.EACCES), errors.Is(errno, unix.EPERM):
		return nil, wrapNoCapability(path, errno)
	case errno != 0:
		return nil, fmt.Errorf("ioctl %s: %w", path, errno)
	}

	// Non-zero return is an NVMe status, not a byte count.
	if err := decodeStatus(int(ret)); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return buf, nil
}

func timeoutMillis(ctx context.Context) uint32 {
	dl, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	ms := time.Until(dl).Milliseconds()
	if ms <= 0 {
		return 1
	}
	if ms > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(ms)
}

var _ Source = (*ioctlSource)(nil)
