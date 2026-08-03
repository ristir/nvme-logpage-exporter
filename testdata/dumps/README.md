# Dump fixtures

Directories other than `synthetic-samsung` are real dumps captured from
production hardware with `nvme_logpage_exporter dump`. Serial numbers are
scrubbed (`SCRUBBED` in `meta.json`, the SN and SUBNQN fields blanked in
`identify.bin`, and every captured buffer scanned for any remaining
verbatim occurrence — see `CONTRIBUTING.md`); model and firmware strings
are left intact.

Directory name is `<vendor>-<model>`, not the original hostname.

| Directory | Model | Firmware | Spec | Sensors | OCP | Pages | Useful for |
|---|---|---|---|---|---|---|---|
| `samsung-pm9a1` | SAMSUNG MZVL2512HCJQ-00B07 | GXA7802Q | 1.3 | 2 | no | 01 02 03 | client drive, no OCP |
| `samsung-datacenter` | SAMSUNG MZQL21T9HCJR-00A07 / MZQLB1T9HAJR-00007 | GDC5902Q / EDA5502Q | 1.4 / 1.2 | 2 / 3 | yes / no | 01 02 03 (+ C0 on nvme0) | two different models in one `meta.json`; model label must be per-controller |
| `micron-3500` | Micron_3500_MTFDKBA512TGD | P8MA002 | 2.0 | 1 | no | 01 02 03 | single sensor, top of the spec range |
| `kioxia-kcd8` | KIOXIA KCD8XRUG1T92 | 0105 | 1.4 | 0 | yes | 01 02 03 C0 | zero sensors with real OCP pages |
| `intel-p4510` | INTEL SSDPE2KX010T8 | VDV10184 | 1.2 | 0 | yes | 01 02 03 C0 | `MaxNamespaces` (NN) = 128, exactly one real namespace exists |
| `dell-p4510` | Dell Express Flash NVMe P4510 1TB SFF | VDV1DP25 | 1.2 | 0 | yes | 01 02 03 C0 | same silicon as `intel-p4510` (VID 0x8086), different firmware, different CCTEMP and NN — a parser must not key on the model string |

`synthetic-samsung` is hand-built, not a real dump; existing unit tests
depend on its exact byte layout. See `internal/collector/realdumps_test.go`
for the test that consumes the fixtures above.
