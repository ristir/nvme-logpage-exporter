package dump

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Walked wholesale: this guard is about any fixture leaking, not about one
// package's parser.
const guardDumpsRoot = "../../testdata/dumps"

// Not captures of real hardware, so exempt.
var guardExcludedDirs = map[string]bool{
	"synthetic-samsung": true,
	"gen":               true,
}

// 10, not 8: at 8 the guard false-positives on clean fixtures.
const guardMinSuspectRunLength = 10

// The character class real serials, models and firmware revisions fall in.
var guardAlnumRun = regexp.MustCompile(`[A-Z0-9]+`)

var guardTokenSplit = regexp.MustCompile(`[^A-Z0-9]+`)

func TestFixturesDoNotLeakIdentifiers(t *testing.T) {
	for _, dir := range guardDumpDirs(t) {
		dir := dir
		t.Run(dir, func(t *testing.T) {
			checkFixtureDirClean(t, filepath.Join(guardDumpsRoot, dir))
		})
	}
}

func guardDumpDirs(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(guardDumpsRoot)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", guardDumpsRoot, err)
	}

	var dirs []string
	for _, e := range entries {
		if !e.IsDir() || guardExcludedDirs[e.Name()] {
			continue
		}
		if _, err := os.Stat(filepath.Join(guardDumpsRoot, e.Name(), "meta.json")); err != nil {
			continue
		}
		dirs = append(dirs, e.Name())
	}
	if len(dirs) == 0 {
		t.Fatal("no real-hardware dump directories found under testdata/dumps")
	}
	return dirs
}

func checkFixtureDirClean(t *testing.T, dir string) {
	t.Helper()

	allowed := guardAllowedTokens(t, filepath.Join(dir, "meta.json"))

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".bin" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, finding := range findLeaks(b, allowed) {
			t.Errorf("%s: %s", path, finding)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
}

// Vendor strings that live in the Identify vendor region and are the same on
// every drive of that model, so they identify a part and not a unit. Verified
// byte-identical across 22 Micron 3400 controllers on eleven hosts.
var guardVendorConstants = []string{
	"210MAR1E0UMP0XDH200UM7P",
}

func guardAllowedTokens(t *testing.T, metaPath string) map[string]bool {
	t.Helper()

	b, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("reading %s: %v", metaPath, err)
	}

	var m struct {
		Controllers []struct {
			Model    string `json:"model"`
			Firmware string `json:"firmware"`
		} `json:"controllers"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parsing %s: %v", metaPath, err)
	}

	allowed := map[string]bool{
		// The scrub marker itself is not a leak.
		scrubbedSerial: true,
	}
	for _, tok := range guardVendorConstants {
		allowed[tok] = true
	}
	for _, c := range m.Controllers {
		for _, tok := range guardTokenSplit.Split(strings.ToUpper(c.Model), -1) {
			if tok != "" {
				allowed[tok] = true
			}
		}
		for _, tok := range guardTokenSplit.Split(strings.ToUpper(c.Firmware), -1) {
			if tok != "" {
				allowed[tok] = true
			}
		}
	}
	return allowed
}

func findLeaks(b []byte, allowed map[string]bool) []string {
	var findings []string

	s := string(b)
	if idx := strings.Index(s, "nqn."); idx >= 0 {
		findings = append(findings, fmt.Sprintf("contains the literal \"nqn.\" (SUBNQN prefix) at byte offset %d", idx))
	}

	for _, loc := range guardAlnumRun.FindAllStringIndex(s, -1) {
		run := s[loc[0]:loc[1]]
		for _, frag := range residualFragments(run, allowed) {
			if len(frag) >= guardMinSuspectRunLength {
				findings = append(findings, fmt.Sprintf(
					"unaccounted-for run %q at byte offset %d (not in any model/firmware string from meta.json)",
					frag, loc[0]))
			}
		}
	}
	return findings
}

func residualFragments(run string, allowed map[string]bool) []string {
	tokens := make([]string, 0, len(allowed))
	for tok := range allowed {
		tokens = append(tokens, tok)
	}
	sort.Slice(tokens, func(i, j int) bool { return len(tokens[i]) > len(tokens[j]) })

	masked := make([]bool, len(run))
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		start := 0
		for {
			idx := strings.Index(run[start:], tok)
			if idx < 0 {
				break
			}
			abs := start + idx
			for i := 0; i < len(tok); i++ {
				masked[abs+i] = true
			}
			start = abs + len(tok)
		}
	}

	var frags []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			frags = append(frags, cur.String())
			cur.Reset()
		}
	}
	for i, m := range masked {
		if m {
			flush()
			continue
		}
		cur.WriteByte(run[i])
	}
	flush()
	return frags
}
