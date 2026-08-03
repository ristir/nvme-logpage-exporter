package nvme

import (
	"errors"
	"io/fs"
	"testing"
	"unsafe"
)

// The struct size is baked into the ioctl number; a drift means EINVAL.
func TestPassthruCmdSize(t *testing.T) {
	if got := unsafe.Sizeof(passthruCmd{}); got != 72 {
		t.Fatalf("sizeof(passthruCmd) = %d, want 72", got)
	}
}

func TestAdminIoctlNumber(t *testing.T) {
	// _IOWR('N', 0x41, sizeof(passthruCmd)).
	const iocInout = uint32(3) << 30 // _IOC_READ|_IOC_WRITE
	want := iocInout | uint32(unsafe.Sizeof(passthruCmd{}))<<16 | uint32('N')<<8 | 0x41
	if nvmeIoctlAdminCmd != want {
		t.Fatalf("nvmeIoctlAdminCmd = %#x, want %#x (derived from sizeof(passthruCmd)=%d)", nvmeIoctlAdminCmd, want, unsafe.Sizeof(passthruCmd{}))
	}
	if want != 0xC0484E41 {
		t.Fatalf("derived expected value %#x != documented 0xC0484E41; sizeof(passthruCmd) must be 72", want)
	}
}

func TestLogPageCDW10(t *testing.T) {
	tests := []struct {
		page uint8
		size int
		want uint32
	}{
		{0x02, 512, 0x007F0002}, // (512/4 - 1) = 127 = 0x7F
		{0x01, 64, 0x000F0001},  // 15
		{0xC0, 512, 0x007F00C0},
	}
	for _, tt := range tests {
		if got := logPageCDW10(tt.page, tt.size); got != tt.want {
			t.Errorf("logPageCDW10(%#x, %d) = %#08x, want %#08x", tt.page, tt.size, got, tt.want)
		}
	}
}

func TestLogPageCDW10RejectsBadSize(t *testing.T) {
	for _, size := range []int{0, 3, 7, -4} {
		if _, err := logPageSize(size); err == nil {
			t.Errorf("size %d should be rejected: it is not positive and a multiple of 4", size)
		}
	}
}

func TestLogPageSizeRejectsNUMDOverflow(t *testing.T) {
	const tooLarge = 4 * (0xFFFF + 2) // NUMD would be 0x10000, one past the max
	if _, err := logPageSize(tooLarge); err == nil {
		t.Errorf("size %d should be rejected: NUMD would overflow 16 bits", tooLarge)
	}

	const maxOK = 4 * (0xFFFF + 1) // NUMD == 0xFFFF, the largest representable value
	if _, err := logPageSize(maxOK); err != nil {
		t.Errorf("size %d should be accepted: NUMD == 0xFFFF fits exactly, got %v", maxOK, err)
	}
}

func TestDecodeStatusUnsupported(t *testing.T) {
	for _, status := range []int{0x0109, 0x4109, 0x02} {
		err := decodeStatus(status)
		if !errors.Is(err, ErrPageUnsupported) {
			t.Errorf("decodeStatus(%#x) = %v, want ErrPageUnsupported", status, err)
		}
	}
}

// A Dell P4510 returns 0x4109: SCT=1 SC=0x09 with the DNR bit set.
func TestDecodeStatusUnsupportedWithDNR(t *testing.T) {
	err := decodeStatus(0x4109) // SCT=1 (Command Specific), SC=0x09, DNR bit set
	if !errors.Is(err, ErrPageUnsupported) {
		t.Errorf("decodeStatus(0x4109) = %v, want ErrPageUnsupported", err)
	}
}

func TestDecodeStatusSameSCDifferentSCT(t *testing.T) {
	err := decodeStatus(0x0209) // SCT=2 (Media and Data Integrity Errors), SC=0x09
	if errors.Is(err, ErrPageUnsupported) {
		t.Errorf("decodeStatus(0x0209) = %v, want an error distinct from ErrPageUnsupported (SCT=2, neither Generic nor Command Specific)", err)
	}
	if err == nil {
		t.Fatal("decodeStatus(0x0209) returned nil, want a non-nil error")
	}
}

func TestDecodeStatusOK(t *testing.T) {
	if err := decodeStatus(0); err != nil {
		t.Errorf("decodeStatus(0) = %v, want nil", err)
	}
}

func TestDecodeStatusOtherIsError(t *testing.T) {
	err := decodeStatus(0x0006) // Internal Device Error
	if err == nil {
		t.Fatal("decodeStatus(0x06) returned nil")
	}
	if errors.Is(err, ErrPageUnsupported) {
		t.Fatal("internal device error mistakenly classified as page-unsupported")
	}
}

// The Linux-only open path is unreachable here, so the wrapper is tested directly.
func TestWrapDeviceOpenPreservesCause(t *testing.T) {
	err := wrapDeviceOpen("/dev/nvme0", fs.ErrNotExist)
	if !errors.Is(err, ErrDeviceOpen) {
		t.Errorf("wrapDeviceOpen(...) = %v, want errors.Is(_, ErrDeviceOpen)", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("wrapDeviceOpen(...) = %v, want errors.Is(_, fs.ErrNotExist)", err)
	}
}

// Same reasoning as the test above, for the EACCES branch.
func TestWrapNoCapabilityPreservesErrno(t *testing.T) {
	err := wrapNoCapability("/dev/nvme0", fs.ErrPermission)
	if !errors.Is(err, ErrNoCapability) {
		t.Errorf("wrapNoCapability(...) = %v, want errors.Is(_, ErrNoCapability)", err)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("wrapNoCapability(...) = %v, want errors.Is(_, fs.ErrPermission)", err)
	}
}
