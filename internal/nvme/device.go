// Package nvme enumerates NVMe devices and reads their log pages.
package nvme

// Controller — NVMe controller, /dev/nvme0. The health log lives here.
type Controller struct {
	Name     string // "nvme0"
	DevPath  string // "/dev/nvme0"
	Model    string
	Firmware string
	Serial   string

	// /sys/class/nvme/nvmeN/state. A non-live controller refuses admin commands.
	State string
}

// Namespace — block device. Never share a metric label with Controller.
type Namespace struct {
	Name        string // "nvme0n1"
	Controller  string // "nvme0"
	SizeBytes   uint64
	SectorBytes uint64
}
