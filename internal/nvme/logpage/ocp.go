package logpage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// IDOCPSmart is the OCP SMART / Health Information Extended log page.
const IDOCPSmart uint8 = 0xC0

// OCPSmartSize is the fixed size of page 0xC0.
const OCPSmartSize = 512

// AFD514C9-7C6F-4F9C-A4F2-BFEA2810AFC5, little-endian. Intel and Dell answer
// 0xC0 with their own data and a zero GUID that decodes into plausible wear.
var ocpGUID = [16]byte{
	0xc5, 0xaf, 0x10, 0x28, 0xea, 0xbf, 0xf2, 0xa4,
	0x9c, 0x4f, 0x6f, 0x7c, 0xc9, 0x14, 0xd5, 0xaf,
}

// ErrNotOCP means 0xC0 was readable but is not an OCP log. Routine.
var ErrNotOCP = errors.New("page 0xC0 is not an OCP extended health log")

// OCPSmart is the parsed page 0xC0; any field may read all ones for
// "unimplemented". Layout verified 26 of 26 against nvme ocp smart-add-log.
type OCPSmart struct {
	Version uint16

	// Bytes that reached the NAND; over the host counter this is amplification.
	PhysicalMediaWrittenBytes OptU128
	PhysicalMediaReadBytes    OptU128

	BadUserNANDBlocksRaw          OptU64
	BadUserNANDBlocksNormalized   OptU64
	BadSystemNANDBlocksRaw        OptU64
	BadSystemNANDBlocksNormalized OptU64

	XORRecoveryCount        OptU64
	UncorrectableReadErrors OptU64
	SoftECCErrors           OptU64
	E2EDetectedErrors       OptU64
	E2ECorrectedErrors      OptU64

	SystemDataPercentUsed OptU64
	RefreshCount          OptU64
	MaxUserDataEraseCount OptU64
	MinUserDataEraseCount OptU64

	// One byte wide: saturates at 255 instead of wrapping.
	ThermalThrottleEvents        OptU64
	ThermalThrottleStatusPercent OptU64

	PCIeCorrectableErrors OptU64
	IncompleteShutdowns   OptU64
	PercentFreeBlocks     OptU64

	// The spec calls this a percentage; real drives report 162 and 231.
	CapacitorHealth OptU64

	UnalignedIO     OptU64
	SecurityVersion OptU64

	// OCP NUSE: bytes in use on namespace 1.
	NamespaceUtilizationBytes OptU64

	PLPStartCount OptU128

	// Units of 10^9 bytes.
	EnduranceEstimateGB OptU128
}

// ParseOCPSmart returns ErrNotOCP when the GUID does not match.
func ParseOCPSmart(b []byte) (*OCPSmart, error) {
	if len(b) < OCPSmartSize {
		return nil, fmt.Errorf("page 0xC0: got %d bytes, need at least %d", len(b), OCPSmartSize)
	}
	if !bytes.Equal(b[496:512], ocpGUID[:]) {
		return nil, ErrNotOCP
	}

	return &OCPSmart{
		Version: binary.LittleEndian.Uint16(b[494:496]),

		PhysicalMediaWrittenBytes: readOptUint128(b, 0),
		PhysicalMediaReadBytes:    readOptUint128(b, 16),

		BadUserNANDBlocksRaw:          readOptUint(b, 32, 6),
		BadUserNANDBlocksNormalized:   readOptUint(b, 38, 2),
		BadSystemNANDBlocksRaw:        readOptUint(b, 40, 6),
		BadSystemNANDBlocksNormalized: readOptUint(b, 46, 2),

		XORRecoveryCount:        readOptUint(b, 48, 8),
		UncorrectableReadErrors: readOptUint(b, 56, 8),
		SoftECCErrors:           readOptUint(b, 64, 8),
		E2EDetectedErrors:       readOptUint(b, 72, 4),
		E2ECorrectedErrors:      readOptUint(b, 76, 4),

		SystemDataPercentUsed: readOptUint(b, 80, 1),
		RefreshCount:          readOptUint(b, 81, 7),
		MaxUserDataEraseCount: readOptUint(b, 88, 4),
		MinUserDataEraseCount: readOptUint(b, 92, 4),

		ThermalThrottleEvents:        readOptUint(b, 96, 1),
		ThermalThrottleStatusPercent: readOptUint(b, 97, 1),

		PCIeCorrectableErrors: readOptUint(b, 104, 8),
		IncompleteShutdowns:   readOptUint(b, 112, 4),
		PercentFreeBlocks:     readOptUint(b, 120, 1),
		CapacitorHealth:       readOptUint(b, 128, 2),
		UnalignedIO:           readOptUint(b, 136, 8),
		SecurityVersion:       readOptUint(b, 144, 8),

		NamespaceUtilizationBytes: readOptUint(b, 152, 8),
		PLPStartCount:             readOptUint128(b, 160),
		EnduranceEstimateGB:       readOptUint128(b, 176),
	}, nil
}
