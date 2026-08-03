package nvme

import (
	"strings"
	"testing"
)

func TestOpenReplaySpec(t *testing.T) {
	dir := fakeDumpDir(t)

	src, err := Open("dir:" + dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, ok := src.(*replaySource); !ok {
		t.Fatalf("Open returned %T, want *replaySource", src)
	}
}

func TestOpenRejectsUnknownScheme(t *testing.T) {
	_, err := Open("http://example.invalid/dumps")
	if err == nil {
		t.Fatal("Open accepted an unknown scheme")
	}
	if !strings.Contains(err.Error(), "auto") {
		t.Errorf("error message should hint at the accepted values: %v", err)
	}
}

func TestOpenRejectsEmpty(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("Open accepted an empty string")
	}
}
