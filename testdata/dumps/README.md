# Dump fixtures

Every directory except `synthetic-samsung`, `synthetic-ocp` and `gen` holds
a real dump captured from production hardware with
`nvme_logpage_exporter dump`. Serial numbers are
scrubbed (`SCRUBBED` in `meta.json`, the SN and SUBNQN fields blanked in
`identify.bin`, and every captured buffer scanned for any remaining
verbatim occurrence — see `CONTRIBUTING.md`); model and firmware strings
are left intact.

Directory name is `<vendor>-<model>`, not the original hostname.

| Directory | Model | Firmware | Spec | Sensors | OCP | Pages | Useful for |
|---|---|---|---|---|---|---|---|
| `samsung-pm9a1` | SAMSUNG MZVL2512HCJQ-00B07 | GXA7802Q | 1.3 | 2 | no | 01 02 03 06 | client drive, no OCP; the only fixture whose self-test log has entries — 11 on nvme0 and 3 on nvme1, mixing passes with runs a controller reset aborted |
| `samsung-datacenter` | SAMSUNG MZQL21T9HCJR-00A07 / MZQLB1T9HAJR-00007 | GDC5902Q / EDA5502Q | 1.4 / 1.2 | 2 / 3 | yes / no | 01 02 03 (+ C0 on nvme0) | two different models in one `meta.json`; model label must be per-controller |
| `micron-3500` | Micron_3500_MTFDKBA512TGD | P8MA002 | 2.0 | 1 | no | 01 02 03 | single sensor, top of the spec range |
| `kioxia-kcd8` | KIOXIA KCD8XRUG1T92 | 0105 | 1.4 | 0 | yes | 01 02 03 06 C0 | zero sensors with real OCP pages; serves page 06 with an empty log, which must not read as twenty passes |
| `intel-p4510` | INTEL SSDPE2KX010T8 | VDV10184 | 1.2 | 0 | yes | 01 02 03 C0 | `MaxNamespaces` (NN) = 128, exactly one real namespace exists |
| `dell-p4510` | Dell Express Flash NVMe P4510 1TB SFF | VDV1DP25 | 1.2 | 0 | yes | 01 02 03 C0 | same silicon as `intel-p4510` (VID 0x8086), different firmware, different CCTEMP and NN — a parser must not key on the model string |
| `micron-3400` | Micron_3400_MTFDKBA512TFH | P7MU002 | 1.4 | 1 | no | 01 02 03 09 | the only fixture serving the endurance group log (09); ELPE 255, so the log holds 256 entries |
| `samsung-worn-degraded` | SAMSUNG MZVLB512HAJQ-00000 | EXA7301Q | 1.2 | 2 | no | 01 02 03 C0 | 133% and 135% used with `reliability_degraded` set — endurance above 1.0 must survive the ratio conversion |
| `samsung-errorlog-full` | SAMSUNG MZVKW512HMJP-00000 | CXA7500Q | 1.2 | 2 | no | 01 02 03 C0 | the error log is full: all 64 entries the drive retains are populated, where a fixed 512-byte read would have seen 8 |
| `samsung-saturated` | SAMSUNG MZVLB512HAJQ-00000 | EXA7301Q | 1.2 | 2 | no | 01 02 03 C0 | Percentage Used reads 255, the ceiling of the one-byte field, and Controller Busy Time is 712 billion minutes — both confirmed against `nvme-cli`, so neither may be clamped |
| `samsung-hot-sensor` | SAMSUNG MZVLB512HAJQ-00000 | EXA7301Q | 1.2 | 2 | no | 01 02 03 C0 | sensor 2 sits at 89 and 90 C, above both WCTEMP and CCTEMP, while the composite reads 54 and 58 and the over-temperature counters stay zero — the thresholds govern the composite alone |

The four Samsung fixtures above answer page 0xC0 with data of their own
whose GUID is not the OCP one, the same way `intel-p4510` and `dell-p4510`
do, so the parser must reject them on the GUID rather than on the model.

Page 06 is present in two fixtures only. It was captured from a second drive
of the same model and firmware, not from the drive the rest of the directory
came from, because no drive that already had a fixture also had self-test
entries. Every other fixture omits the file, which the replay source treats as
the drive not serving the page — the same answer the real transport gives.

`synthetic-samsung` and `synthetic-ocp` are hand-built, not real dumps;
unit tests depend on their exact byte layout, and `gen` holds the generator
that writes them. See `internal/collector/realdumps_test.go`
for the test that consumes the fixtures above.
