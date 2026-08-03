package nvme

import (
	"fmt"
	"strings"
)

// Open creates a source: "auto" for the ioctl source (Linux only), or
// "dir:<path>" to replay a dump. No URL support: no network code here.
func Open(spec string) (Source, error) {
	switch {
	case spec == "auto":
		return NewIoctl(NewSysFS())

	case strings.HasPrefix(spec, "dir:"):
		dir := strings.TrimPrefix(spec, "dir:")
		if dir == "" {
			return nil, fmt.Errorf("source dir: is missing a directory path")
		}
		return NewReplay(dir)

	default:
		return nil, fmt.Errorf("unknown source %q: valid values are auto or dir:<path>", spec)
	}
}
