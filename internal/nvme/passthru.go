package nvme

import (
	"errors"
	"fmt"
)

var (
	// ErrDeviceOpen means the file would not open: permissions or a udev rule.
	ErrDeviceOpen = errors.New("failed to open device file")

	// ErrNoCapability means the ioctl was refused: a missing CAP_SYS_ADMIN.
	ErrNoCapability = errors.New("missing CAP_SYS_ADMIN")
)

// The struct size is baked into the number: a mismatch is EINVAL every time.
const nvmeIoctlAdminCmd = 0xC0484E41

// NVMe admin command opcodes.
const (
	opGetLogPage uint8 = 0x02
	opIdentify   uint8 = 0x06
)

// nsidAll addresses every namespace: the health log is per controller.
const nsidAll uint32 = 0xFFFFFFFF

// cnsController — Identify Controller Data Structure.
const cnsController uint32 = 0x01

// Status Code Types (bits 10:8 of the status field).
const (
	sctGeneric         = 0x0 // Generic Command Status
	sctCommandSpecific = 0x1 // Command Specific Status
)

const (
	// Invalid Field in Command, Generic status.
	scInvalidField = 0x02

	// Command Specific, not Generic. Measured on a Micron 3500 and a Dell P4510.
	scInvalidLogPage = 0x09
)

// Mirrors struct nvme_passthru_cmd; field order and size must not change.
type passthruCmd struct {
	opcode      uint8
	flags       uint8
	rsvd1       uint16
	nsid        uint32
	cdw2        uint32
	cdw3        uint32
	metadata    uint64
	addr        uint64
	metadataLen uint32
	dataLen     uint32
	cdw10       uint32
	cdw11       uint32
	cdw12       uint32
	cdw13       uint32
	cdw14       uint32
	cdw15       uint32
	timeoutMs   uint32
	result      uint32
}

// Dwords minus one. Sizes needing NUMDU are rejected, not truncated.
func logPageSize(size int) (uint32, error) {
	if size <= 0 || size%4 != 0 {
		return 0, fmt.Errorf("log page size %d: must be positive and a multiple of 4", size)
	}
	numd := uint32(size/4 - 1)
	if numd > 0xFFFF {
		return 0, fmt.Errorf("log page size %d: NUMDU is not supported, maximum size is 256 KiB", size)
	}
	return numd, nil
}

// Caller must have validated size with logPageSize.
func logPageCDW10(pageID uint8, size int) uint32 {
	numd, err := logPageSize(size)
	if err != nil {
		return 0
	}
	return uint32(pageID) | (numd&0xFFFF)<<16
}

// Both stay in the chain: ENOENT is benign, EACCES needs a udev rule.
func wrapDeviceOpen(path string, cause error) error {
	return fmt.Errorf("%s: %w: %w", path, ErrDeviceOpen, cause)
}

func wrapNoCapability(path string, errno error) error {
	return fmt.Errorf("%s: %w: %w", path, ErrNoCapability, errno)
}

// Positive ioctl return is a status: bits 10:8 type, 7:0 code. Two encodings
// mean unsupported because controllers disagree.
func decodeStatus(status int) error {
	if status == 0 {
		return nil
	}
	sct := (status >> 8) & 0x7
	sc := status & 0xFF

	unsupported := (sct == sctCommandSpecific && sc == scInvalidLogPage) ||
		(sct == sctGeneric && sc == scInvalidField)
	if unsupported {
		return fmt.Errorf("status SCT=%#x SC=%#x: %w", sct, sc, ErrPageUnsupported)
	}
	return fmt.Errorf("NVMe returned status SCT=%#x SC=%#x", sct, sc)
}
