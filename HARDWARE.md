# Verified hardware

Every drive listed here was measured in production, not inferred from a
datasheet. The numbers come from live `/metrics` output across 386 hosts and
876 devices, 29 models from 5 vendors.

**Full support** means the controller serves all four log pages the exporter
reads — 0x01, 0x02, 0x03 and OCP 0xC0 — and implements every one of the 24
OCP fields. That yields 57-58 distinct metrics per device. A drive without
OCP yields 32-34.

## This list is not a compatibility requirement

**A drive missing from these tables is not unsupported.** This is what we
happened to have in production, not a set of drives the exporter was
written against. There is no allowlist in the code and no model matching
anywhere: the exporter asks each controller what it serves and exports
whatever comes back.

So an unlisted drive will almost certainly work. What varies is how much
you get — a controller that serves fewer log pages exports fewer metrics
and says so in `nvme_logpage_supported`, one series per page, per device.

If it does not work, or the numbers look wrong, that is a bug worth
reporting: send a dump. The same goes if it works fine — a dump from a
model nobody here owns is how it gets into the tests and stays working.
See [Adding your drive](#adding-your-drive).

## Full support

| Model | Vendor | NVMe | Firmware | OCP | Devices |
|---|---|---|---|---|---|
| `MTFDKCC960TGP-1BK1DABYY` | Micron (0x1344) | 2.0 | E3MQ005 | v3 | 54 |
| `SAMSUNG MZQL27T6HBLA-00A07` | Samsung (0x144d) | 1.4 | GDC5902Q, GDC5A02Q | v2 | 44 |
| `Micron_7450_MTFDKCC960TFR` | Micron (0x1344) | 1.4 | E2MU200 | v2 | 32 |
| `KIOXIA KCD8XRUG1T92` | KIOXIA (0x1e0f) | 1.4 | 0105 | v3 | 20 |
| `SAMSUNG MZQL21T9HCJR-00A07` | Samsung (0x144d) | 1.4 | GDC5602Q, GDC5902Q | v2 | 14 |
| `KIOXIA KCD81RUG7T68` | KIOXIA (0x1e0f) | 1.4 | 8003 | v3 | 12 |
| `MTFDKCC1T9TGP-1BK1DABYY` | Micron (0x1344) | 2.0 | E3MQ000 | v3 | 4 |
| `KIOXIA KCD8XRUG15T3` | KIOXIA (0x1e0f) | 1.4 | 0105 | v3 | 10 |
| `SAMSUNG MZQL23T8HCLS-00A07` | Samsung (0x144d) | 1.4 | GDC5A02Q | v2 | 6 |
| `Micron_7450_MTFDKCC7T6TFR` | Micron (0x1344) | 1.4 | E2MU200 | v2 | 3 |

All ten implement 24/24 OCP fields. Two caveats, both on the KIOXIA drives
and both visible as absent series rather than wrong values:

- no per-sensor temperatures on any KIOXIA here, so
  `nvme_logpage_temperature_celsius` is not emitted — only the composite
  reading. The Micron and Samsung entries above report two or three sensors.
- the system-area NAND block counters read all ones, so
  `bad_nand_blocks_total` and `bad_nand_blocks_normalized` carry
  `area="user"` only. Every Micron and Samsung here reports both areas.

## Partial support: no OCP log

These serve 0x01, 0x02 and 0x03. Everything in the OCP section of the
README is unavailable on them, including true write amplification.

| Model | Vendor | NVMe | Firmware | Devices |
|---|---|---|---|---|
| `SAMSUNG MZVL2512HCJQ-00B07` | Samsung (0x144d) | 1.3 | GXA7802Q | 300 |
| `SAMSUNG MZVL2512HCJQ-00B00` | Samsung (0x144d) | 1.3 | GXA7801Q | 134 |
| `Micron_3500_MTFDKBA512TGD` | Micron (0x1344) | 2.0 | P8MA002 | 56 |
| `SAMSUNG MZQLB960HAJR-00007` | Samsung (0x144d) | 1.2 | EDA5502Q | 27 |
| `Micron_3400_MTFDKBA512TFH` | Micron (0x1344) | 1.4 | P7MU002 | 20 |
| `SAMSUNG MZQLB1T9HAJR-00007` | Samsung (0x144d) | 1.2 | EDA5502Q | 12 |
| `SAMSUNG MZQLW960HMJP-00003` | Samsung (0x144d) | 1.2 | CXV8601Q | 10 |
| `SAMSUNG MZVLB1T0HALR-00000` | Samsung (0x144d) | 1.2 | EXA7301Q | 10 |
| `SAMSUNG MZVLB512HAJQ-00000` | Samsung (0x144d) | 1.2 | EXA7301Q | 9 |
| `Micron_3500_MTFDKBA512TGD-1BK1AABYY` | Micron (0x1344) | 2.0 | P8MA002 | 8 |
| `Dell Express Flash NVMe P4510 1TB SFF` | Intel (0x8086) | 1.2 | VDV1DP25 | 8 |
| `SAMSUNG MZVKW512HMJP-00000` | Samsung (0x144d) | 1.2 | CXA7500Q | 6 |
| `KXG60ZNV1T02 TOSHIBA` | Toshiba (0x1179) | 1.3 | AGGA4104 | 5 |
| `SAMSUNG MZVL21T0HCLR-00B00` | Samsung (0x144d) | 1.3 | GXA7601Q, GXA7801Q | 5 |
| `SAMSUNG MZVL2512HDJD-00B07` | Samsung (0x144d) | 1.3 | GXD7102Q | 4 |
| `INTEL SSDPE2KX010T8` | Intel (0x8086) | 1.2 | VDV10184 | 4 |
| `SAMSUNG MZVLB1T0HBLR-00000` | Samsung (0x144d) | 1.3 | EXF7201Q | 3 |
| `KXG50ZNV512G TOSHIBA` | Toshiba (0x1179) | 1.2.1 | AAGA4106 | 2 |
| `SAMSUNG MZQLW1T9HMJP-00003` | Samsung (0x144d) | 1.2 | CXV8601Q | 1 |

Two distinct failure modes hide behind that single column, and the exporter
handles them differently:

- **Refusal.** The controller answers page 0xC0 with "Invalid Log Page"
  (SCT=1, SC=0x09). Each refusal appends an entry to the drive's own error
  log, so the answer is asked for once per process and remembered.
- **Foreign data.** The Intel and Dell P4510 and every Samsung client drive
  above answer 0xC0 with 512 bytes whose GUID is not the OCP one. Nothing is
  appended to the error log, and the page is rejected at the GUID check.

## Model name is not enough

The Intel and Dell P4510 entries above are the same silicon. The
Dell-branded firmware reports `NN=1` and no Namespace Management support,
while the Intel one reports `NN=128` and supports it. OEM firmware changes
what a controller exposes; do not infer capability from a model number.

`Micron_3500_MTFDKBA512TGD` and `Micron_3500_MTFDKBA512TGD-1BK1AABYY` are
listed separately for the same reason: the OEM suffix is part of the model
string the controller returns, and nothing guarantees the two behave alike.

## Known gaps

- **OCP layout is read up to v3.** Versions 2 and 3 were verified
  byte-identical here. OCP extends the page additively into reserved
  space, so a v5 drive parses correctly for every field the exporter
  reads — but the two fields v5 adds at offsets 192-207, PCIe retraining
  count and power state change count, are not read.
- **Multipath and multi-namespace subsystems are untested.** All 804 devices
  measured have exactly one namespace, which is also why the endurance group
  log (0x09) is not exported: the one model that serves it repeats what page
  0x02 already says.
- **Counters are reported as the drive gives them.** Five devices here report
  a Controller Busy Time larger than their own power-on time, one by a factor
  of 250,000; two report Percentage Used at 255, the ceiling of the field.
  `nvme-cli` returns the same values, so nothing is clamped or corrected.

## Adding your drive

Run `nvme_logpage_exporter dump --out ./dump` and open an issue with the
result. Serials are scrubbed by default, from the Identify serial field,
from SUBNQN, and by a scan of every buffer written. Attach your own
`nvme smart-log` and `nvme id-ctrl` output as ground truth; the dump then
becomes a fixture with expected values, and the model joins a table that
regression tests keep honest. See [`CONTRIBUTING.md`](CONTRIBUTING.md).
