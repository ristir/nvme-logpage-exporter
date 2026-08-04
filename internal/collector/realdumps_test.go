package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ristir/nvme-logpage-exporter/internal/nvme"
	"github.com/ristir/nvme-logpage-exporter/internal/nvme/logpage"
)

const realDumpsRoot = "../../testdata/dumps"

// Excluded: hand-built input for the unit tests, not real hardware.
const syntheticDumpDir = "synthetic-samsung"

// Ground truth from the fleet summary, not read back out of the fixture.
type ctrlExpectation struct {
	sensors       int    // count of present temperature sensors
	maxNamespaces uint32 // NN field: max namespaces supported, not the real count
	namespaces    int    // real namespace count, from meta.json
}

var realDumpExpectations = map[string]map[string]ctrlExpectation{
	"samsung-hot-sensor": {
		// Sensor 2 sits at 89 and 90 C, above both WCTEMP and CCTEMP, while the
		// composite reads 54 and 58 and the over-temperature counters stay zero:
		// the thresholds apply to the composite alone.
		"nvme0": {sensors: 2, maxNamespaces: 1, namespaces: 1},
		"nvme1": {sensors: 2, maxNamespaces: 1, namespaces: 1},
	},
	"samsung-worn-degraded": {
		// SAMSUNG MZVLB512HAJQ, 133% and 135% used with reliability_degraded set.
		"nvme0": {sensors: 2, maxNamespaces: 1, namespaces: 1},
		"nvme1": {sensors: 2, maxNamespaces: 1, namespaces: 1},
	},
	"samsung-errorlog-full": {
		// SAMSUNG MZVKW512HMJP, the only fixture whose error log fills all
		// eight entries the exporter reads.
		"nvme0": {sensors: 2, maxNamespaces: 1, namespaces: 1},
		"nvme1": {sensors: 2, maxNamespaces: 1, namespaces: 1},
	},
	"samsung-saturated": {
		// Percentage Used reads 255, the maximum the one-byte field holds, and
		// Controller Busy Time is 712 billion minutes. Both confirmed by nvme-cli.
		"nvme0": {sensors: 2, maxNamespaces: 1, namespaces: 1},
		"nvme1": {sensors: 2, maxNamespaces: 1, namespaces: 1},
	},
	"micron-3400": {
		// Micron_3400_MTFDKBA512TFH, healthy, one sensor, ELPE 255.
		"nvme0": {sensors: 1, maxNamespaces: 1, namespaces: 1},
		"nvme1": {sensors: 1, maxNamespaces: 1, namespaces: 1},
	},
	"samsung-pm9a1": {
		// SAMSUNG MZVL2512HCJQ-00B07, client drive, no OCP, spec 1.3.
		"nvme0": {sensors: 2, maxNamespaces: 1, namespaces: 1},
		"nvme1": {sensors: 2, maxNamespaces: 1, namespaces: 1},
	},
	"samsung-datacenter": {
		// Two Samsung models in one meta.json; only MZQL21T9HCJR has OCP.
		"nvme0": {sensors: 2, maxNamespaces: 32, namespaces: 1},
		"nvme1": {sensors: 3, maxNamespaces: 1, namespaces: 1},
	},
	"micron-3500": {
		// Micron_3500_MTFDKBA512TGD, spec 2.0, no OCP.
		"nvme0": {sensors: 1, maxNamespaces: 1, namespaces: 1},
		"nvme1": {sensors: 1, maxNamespaces: 1, namespaces: 1},
	},
	"kioxia-kcd8": {
		// KIOXIA KCD8XRUG1T92, zero sensors, OCP present, spec 1.4.
		"nvme0": {sensors: 0, maxNamespaces: 64, namespaces: 1},
		"nvme1": {sensors: 0, maxNamespaces: 64, namespaces: 1},
	},
	"intel-p4510": {
		// INTEL SSDPE2KX010T8, zero sensors, OCP present.
		"nvme0": {sensors: 0, maxNamespaces: 128, namespaces: 1},
		"nvme1": {sensors: 0, maxNamespaces: 128, namespaces: 1},
	},
	"dell-p4510": {
		// Same silicon as intel-p4510; different firmware moves NN and CCTEMP.
		"nvme0": {sensors: 0, maxNamespaces: 1, namespaces: 1},
		"nvme1": {sensors: 0, maxNamespaces: 1, namespaces: 1},
	},
	"synthetic-ocp": {
		// Synthetic: no fleet hardware shows non-zero throttling or all-ones fields.
		"nvme0": {sensors: 1, maxNamespaces: 1, namespaces: 1},
	},
}

func realDumpDirs(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(realDumpsRoot)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", realDumpsRoot, err)
	}

	var dirs []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == syntheticDumpDir {
			continue
		}
		if _, err := os.Stat(filepath.Join(realDumpsRoot, e.Name(), "meta.json")); err != nil {
			continue // not a dump directory, e.g. testdata/dumps/gen
		}
		dirs = append(dirs, e.Name())
	}
	return dirs
}

func TestRealDumpsParse(t *testing.T) {
	dirs := realDumpDirs(t)
	if len(dirs) == 0 {
		t.Fatal("no real-hardware dump directories found under testdata/dumps")
	}

	seen := make(map[string]bool, len(dirs))

	for _, dir := range dirs {
		dir := dir
		t.Run(dir, func(t *testing.T) {
			ctrlExp, ok := realDumpExpectations[dir]
			if !ok {
				t.Fatalf("testdata/dumps/%s has no entry in realDumpExpectations; add one", dir)
			}
			seen[dir] = true

			r, err := nvme.NewReplay(filepath.Join(realDumpsRoot, dir))
			if err != nil {
				t.Fatalf("NewReplay: %v", err)
			}

			ctrls, err := r.Controllers()
			if err != nil {
				t.Fatalf("Controllers: %v", err)
			}
			if len(ctrls) == 0 {
				t.Fatal("meta.json lists no controllers")
			}

			testedControllers := make(map[string]bool, len(ctrls))

			for _, c := range ctrls {
				c := c
				t.Run(c.Name, func(t *testing.T) {
					exp, ok := ctrlExp[c.Name]
					if !ok {
						t.Fatalf("no expectation entry for controller %q in %q; add one", c.Name, dir)
					}
					testedControllers[c.Name] = true

					if c.Serial != "SCRUBBED" {
						t.Errorf("Serial = %q, want %q", c.Serial, "SCRUBBED")
					}

					idBytes, err := r.Identify(context.Background(), c.Name)
					if err != nil {
						t.Fatalf("Identify: %v", err)
					}
					id, err := logpage.ParseIdentify(idBytes)
					if err != nil {
						t.Fatalf("ParseIdentify: %v", err)
					}

					if id.Serial != "SCRUBBED" {
						t.Errorf("Identify.Serial = %q, want %q", id.Serial, "SCRUBBED")
					}
					if id.MaxNamespaces != exp.maxNamespaces {
						t.Errorf("MaxNamespaces = %d, want %d", id.MaxNamespaces, exp.maxNamespaces)
					}
					if id.WarnTempKelvin == 0 {
						t.Error("WarnTempKelvin = 0, want a non-zero WCTEMP")
					}
					if id.CritTempKelvin == 0 {
						t.Error("CritTempKelvin = 0, want a non-zero CCTEMP")
					}
					if id.CritTempKelvin < id.WarnTempKelvin {
						t.Errorf("CritTempKelvin (%d) < WarnTempKelvin (%d)", id.CritTempKelvin, id.WarnTempKelvin)
					}

					smartBytes, err := r.LogPage(context.Background(), c.Name, logpage.IDSmart, logpage.SmartLogSize)
					if err != nil {
						t.Fatalf("LogPage(0x02): %v", err)
					}
					smart, err := logpage.ParseSmart(smartBytes)
					if err != nil {
						t.Fatalf("ParseSmart: %v", err)
					}
					if got := len(smart.PresentSensors()); got != exp.sensors {
						t.Errorf("PresentSensors count = %d, want %d", got, exp.sensors)
					}

					ns, err := r.Namespaces(c.Name)
					if err != nil {
						t.Fatalf("Namespaces: %v", err)
					}
					if got := len(ns); got != exp.namespaces {
						t.Errorf("real namespace count = %d, want %d (MaxNamespaces=%d)",
							got, exp.namespaces, id.MaxNamespaces)
					}
				})
			}

			for name := range ctrlExp {
				if !testedControllers[name] {
					t.Errorf("expectation entry for controller %q was never matched against meta.json", name)
				}
			}
		})
	}

	for dir := range realDumpExpectations {
		if !seen[dir] {
			t.Errorf("realDumpExpectations has entry %q but testdata/dumps/%s does not exist", dir, dir)
		}
	}
}

// Ground truth from `nvme ocp smart-add-log`, not read back out of the fixture.
type ocpExpectation struct {
	present       bool   // does the fixture carry a valid OCP page at all
	version       uint16 // log page version, when present
	mediaWritten  uint64 // Physical Media Units Written, bytes
	badSystemNAND bool   // is Bad System NAND Blocks implemented
}

var ocpExpectations = map[string]map[string]ocpExpectation{
	"kioxia-kcd8": {
		// KIOXIA KCD8XRUG1T92. Reports Bad System NAND Blocks as all ones.
		"nvme0": {present: true, version: 3, mediaWritten: 656577089470464, badSystemNAND: false},
		"nvme1": {present: true, version: 3, mediaWritten: 663478961602560, badSystemNAND: false},
	},
	"samsung-datacenter": {
		// Two controllers disagreeing about OCP inside one dump.
		"nvme0": {present: true, version: 2, mediaWritten: 195459011571712, badSystemNAND: true},
		"nvme1": {present: false},
	},
	// Intel and Dell answer page 0xC0 with data of their own and a zero GUID.
	"intel-p4510": {
		"nvme0": {present: false},
		"nvme1": {present: false},
	},
	"dell-p4510": {
		"nvme0": {present: false},
		"nvme1": {present: false},
	},
	// These Samsung client drives answer 0xC0 with data of their own, the same
	// way the Intel and Dell above do.
	"samsung-hot-sensor": {
		"nvme0": {present: false},
		"nvme1": {present: false},
	},
	"samsung-worn-degraded": {
		"nvme0": {present: false},
		"nvme1": {present: false},
	},
	"samsung-errorlog-full": {
		"nvme0": {present: false},
		"nvme1": {present: false},
	},
	"samsung-saturated": {
		"nvme0": {present: false},
		"nvme1": {present: false},
	},
	"synthetic-ocp": {
		"nvme0": {present: true, version: 3, mediaWritten: 1 << 50, badSystemNAND: false},
	},
}

func TestRealDumpsOCP(t *testing.T) {
	for dir, ctrls := range ocpExpectations {
		for ctrl, want := range ctrls {
			t.Run(dir+"/"+ctrl, func(t *testing.T) {
				src, err := nvme.NewReplay(filepath.Join(realDumpsRoot, dir))
				if err != nil {
					t.Fatalf("NewReplay: %v", err)
				}

				raw, err := src.LogPage(context.Background(), ctrl, logpage.IDOCPSmart, logpage.OCPSmartSize)
				if err != nil {
					if !want.present && errors.Is(err, nvme.ErrPageUnsupported) {
						return
					}
					t.Fatalf("LogPage 0xC0: %v", err)
				}

				p, err := logpage.ParseOCPSmart(raw)
				if !want.present {
					if !errors.Is(err, logpage.ErrNotOCP) {
						t.Fatalf("err = %v, want ErrNotOCP", err)
					}
					return
				}
				if err != nil {
					t.Fatalf("ParseOCPSmart: %v", err)
				}

				if p.Version != want.version {
					t.Errorf("Version = %d, want %d", p.Version, want.version)
				}
				if want.mediaWritten != 0 {
					if !p.PhysicalMediaWrittenBytes.Present {
						t.Fatal("PhysicalMediaWrittenBytes absent")
					}
					if p.PhysicalMediaWrittenBytes.Value.Lo != want.mediaWritten ||
						p.PhysicalMediaWrittenBytes.Value.Hi != 0 {
						t.Errorf("PhysicalMediaWrittenBytes = %+v, want %d",
							p.PhysicalMediaWrittenBytes.Value, want.mediaWritten)
					}
				}
				if p.BadSystemNANDBlocksRaw.Present != want.badSystemNAND {
					t.Errorf("BadSystemNANDBlocksRaw.Present = %v, want %v",
						p.BadSystemNANDBlocksRaw.Present, want.badSystemNAND)
				}
			})
		}
	}
}

func TestRealDumpsOCPExpectationsAreComplete(t *testing.T) {
	entries, err := os.ReadDir(realDumpsRoot)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	for _, ent := range entries {
		if !ent.IsDir() || ent.Name() == syntheticDumpDir || ent.Name() == "gen" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(realDumpsRoot, ent.Name(), "nvme*", "logpage-0xc0.bin"))
		if err != nil {
			t.Fatalf("Glob: %v", err)
		}
		if len(matches) == 0 {
			continue
		}
		if _, ok := ocpExpectations[ent.Name()]; !ok {
			t.Errorf("fixture %q has a 0xC0 page but no entry in ocpExpectations", ent.Name())
		}
	}
}

// Thermal throttling reads zero on all fourteen fleet dumps.
func TestSyntheticOCPThrottlingAndUnimplementedFields(t *testing.T) {
	src, err := nvme.NewReplay(filepath.Join(realDumpsRoot, "synthetic-ocp"))
	if err != nil {
		t.Fatalf("NewReplay: %v", err)
	}

	raw, err := src.LogPage(context.Background(), "nvme0", logpage.IDOCPSmart, logpage.OCPSmartSize)
	if err != nil {
		t.Fatalf("LogPage: %v", err)
	}
	p, err := logpage.ParseOCPSmart(raw)
	if err != nil {
		t.Fatalf("ParseOCPSmart: %v", err)
	}

	if !p.ThermalThrottleEvents.Present || p.ThermalThrottleEvents.Value != 7 {
		t.Errorf("ThermalThrottleEvents = %+v, want present 7", p.ThermalThrottleEvents)
	}
	if !p.ThermalThrottleStatusPercent.Present || p.ThermalThrottleStatusPercent.Value != 25 {
		t.Errorf("ThermalThrottleStatusPercent = %+v, want present 25", p.ThermalThrottleStatusPercent)
	}
	for _, f := range []struct {
		name string
		v    logpage.OptU64
	}{
		{"BadSystemNANDBlocksRaw", p.BadSystemNANDBlocksRaw},
		{"CapacitorHealth", p.CapacitorHealth},
		{"SecurityVersion", p.SecurityVersion},
	} {
		if f.v.Present {
			t.Errorf("%s: Present = true, want false", f.name)
		}
	}
}

// firmwareExpectation is ground truth from the source hardware.
type firmwareExpectation struct {
	active    int
	next      int
	populated int
	revisions map[int]string
}

var firmwareExpectations = map[string]map[string]firmwareExpectation{
	"dell-p4510": {
		"nvme0": {active: 2, next: 0, populated: 2, revisions: map[int]string{1: "VDV1DP23", 2: "VDV1DP25"}},
		"nvme1": {active: 2, next: 0, populated: 2, revisions: map[int]string{1: "VDV1DP23", 2: "VDV1DP25"}},
	},
	"kioxia-kcd8": {
		"nvme0": {active: 1, next: 0, populated: 3, revisions: map[int]string{1: "0105", 2: "0105", 3: "0105"}},
	},
	"micron-3500": {
		"nvme0": {active: 1, next: 0, populated: 1, revisions: map[int]string{1: "P8MA002"}},
	},
	"samsung-datacenter": {
		"nvme0": {active: 2, next: 0, populated: 2, revisions: map[int]string{1: "GDC5602Q", 2: "GDC5902Q"}},
		"nvme1": {active: 2, next: 0, populated: 2, revisions: map[int]string{1: "EDA5202Q", 2: "EDA5502Q"}},
	},
	"intel-p4510": {
		"nvme0": {active: 1, next: 0, populated: 1, revisions: map[int]string{1: "VDV10184"}},
	},
}

func TestRealDumpsFirmwareSlots(t *testing.T) {
	for dir, ctrls := range firmwareExpectations {
		for ctrl, want := range ctrls {
			t.Run(dir+"/"+ctrl, func(t *testing.T) {
				src, err := nvme.NewReplay(filepath.Join(realDumpsRoot, dir))
				if err != nil {
					t.Fatalf("NewReplay: %v", err)
				}

				raw, err := src.LogPage(context.Background(), ctrl, logpage.IDFirmwareSlot, logpage.FirmwareSlotSize)
				if err != nil {
					t.Fatalf("LogPage 0x03: %v", err)
				}
				f, err := logpage.ParseFirmwareSlots(raw)
				if err != nil {
					t.Fatalf("ParseFirmwareSlots: %v", err)
				}

				if f.Active != want.active {
					t.Errorf("Active = %d, want %d", f.Active, want.active)
				}
				if f.Next != want.next {
					t.Errorf("Next = %d, want %d", f.Next, want.next)
				}
				if got := f.PopulatedSlots(); len(got) != want.populated {
					t.Errorf("PopulatedSlots() = %d entries, want %d: %+v", len(got), want.populated, got)
				}
				for slot, rev := range want.revisions {
					if got := f.Revisions[slot-1]; got != rev {
						t.Errorf("slot %d = %q, want %q", slot, got, rev)
					}
				}

				// sysfs firmware_rev is by definition the active slot's revision.
				ctrls, err := src.Controllers()
				if err != nil {
					t.Fatalf("Controllers: %v", err)
				}
				found := false
				for _, c := range ctrls {
					if c.Name != ctrl {
						continue
					}
					found = true
					if got := f.Revisions[f.Active-1]; got != c.Firmware {
						t.Errorf("active slot revision = %q, sysfs firmware_rev = %q", got, c.Firmware)
					}
				}
				if !found {
					t.Fatalf("controller %q not found in Controllers(); sysfs oracle check did not run", ctrl)
				}
			})
		}
	}
}

type selfTestExpectation struct {
	entries  int           // written entries, unused slots excluded
	byResult map[uint8]int // result code -> count
	lastPOH  uint64        // power-on hours of the newest entry
}

// Ground truth read off the drives with nvme-cli, not back out of the fixture.
// The KIOXIA logs are empty because nobody has ever run a self-test on them:
// page served and log empty are different facts.
var selfTestExpectations = map[string]map[string]selfTestExpectation{
	"samsung-pm9a1": {
		"nvme0": {entries: 11, byResult: map[uint8]int{0: 7, 2: 4}, lastPOH: 0x2edc},
		"nvme1": {entries: 3, byResult: map[uint8]int{0: 2, 2: 1}},
	},
	"kioxia-kcd8": {
		"nvme0": {entries: 0},
		"nvme1": {entries: 0},
	},
}

func TestRealDumpsSelfTest(t *testing.T) {
	for dir, ctrls := range selfTestExpectations {
		for ctrl, want := range ctrls {
			t.Run(dir+"/"+ctrl, func(t *testing.T) {
				src, err := nvme.NewReplay(filepath.Join(realDumpsRoot, dir))
				if err != nil {
					t.Fatalf("NewReplay: %v", err)
				}

				raw, err := src.LogPage(context.Background(), ctrl, logpage.IDSelfTest, logpage.SelfTestSize)
				if err != nil {
					t.Fatalf("LogPage 0x06: %v", err)
				}
				s, err := logpage.ParseSelfTest(raw)
				if err != nil {
					t.Fatalf("ParseSelfTest: %v", err)
				}

				if len(s.Results) != want.entries {
					t.Fatalf("Results = %d, want %d", len(s.Results), want.entries)
				}
				if s.InProgress != 0 {
					t.Errorf("InProgress = %d, want 0: no test was running when the dump was taken", s.InProgress)
				}

				got := map[uint8]int{}
				for _, r := range s.Results {
					got[r.Result]++
				}
				for res, n := range want.byResult {
					if got[res] != n {
						t.Errorf("result %d: %d entries, want %d (all: %v)", res, got[res], n, got)
					}
				}
				if want.lastPOH != 0 && s.Results[0].PowerOnHours != want.lastPOH {
					t.Errorf("newest entry power-on hours = %#x, want %#x", s.Results[0].PowerOnHours, want.lastPOH)
				}
			})
		}
	}
}

// A page the replay source serves has to parse; one it refuses must refuse
// with ErrPageUnsupported.
func TestRealDumpsSelfTestAbsenceIsClean(t *testing.T) {
	entries, err := os.ReadDir(realDumpsRoot)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	served, absent := 0, 0
	for _, e := range entries {
		if !e.IsDir() || e.Name() == syntheticDumpDir || e.Name() == "gen" {
			continue
		}
		src, err := nvme.NewReplay(filepath.Join(realDumpsRoot, e.Name()))
		if err != nil {
			continue
		}
		ctrls, err := src.Controllers()
		if err != nil {
			t.Fatalf("%s: Controllers: %v", e.Name(), err)
		}
		for _, c := range ctrls {
			raw, err := src.LogPage(context.Background(), c.Name, logpage.IDSelfTest, logpage.SelfTestSize)
			switch {
			case errors.Is(err, nvme.ErrPageUnsupported):
				absent++
			case err != nil:
				t.Errorf("%s/%s: unexpected error: %v", e.Name(), c.Name, err)
			default:
				served++
				if _, err := logpage.ParseSelfTest(raw); err != nil {
					t.Errorf("%s/%s: page served but does not parse: %v", e.Name(), c.Name, err)
				}
			}
		}
	}
	if served == 0 {
		t.Error("no fixture serves page 0x06; the parser has no real-hardware coverage")
	}
	if absent == 0 {
		t.Error("no fixture lacks page 0x06; the absent-page path has no coverage")
	}
	t.Logf("page 0x06: served by %d controllers, absent on %d", served, absent)
}
