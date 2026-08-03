# Verified hardware

Every drive listed here was measured in production, not inferred from a
datasheet. The numbers come from live `/metrics` output across 11 hosts and
28 devices, 12 models from 5 vendors.

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

| Model | Vendor | NVMe | Firmware | OCP | Metrics |
|---|---|---|---|---|---|
| `SAMSUNG MZQL27T6HBLA-00A07` | Samsung (0x144d) | 1.4 | GDC5A02Q | v2 | 58 |
| `Micron_7450_MTFDKCC960TFR` | Micron (0x1344) | 1.4 | E2MU200 | v2 | 58 |
| `MTFDKCC960TGP-1BK1DABYY` | Micron (0x1344) | 2.0 | E3MQ005 | v3 | 58 |
| `KIOXIA KCD8XRUG1T92` | KIOXIA (0x1e0f) | 1.4 | 0105 | v3 | 57 |

All four implement 24/24 OCP fields. Two caveats on the KIOXIA, both
visible as absent series rather than wrong values:

- no per-sensor temperatures, so `nvme_logpage_temperature_celsius` is not
  emitted — only the composite reading
- the system-area NAND block counters read all ones, so
  `bad_nand_blocks_total` and `bad_nand_blocks_normalized` carry
  `area="user"` only

## Partial support: no OCP log

These serve 0x01, 0x02 and 0x03. Everything in the OCP section of the
README is unavailable on them, including true write amplification.

| Model | Vendor | NVMe | Firmware | Metrics |
|---|---|---|---|---|
| `INTEL SSDPE2KX010T8` | Intel (0x8086) | 1.2 | VDV10184 | 32 |
| `Dell Express Flash NVMe P4510 1TB SFF` | Intel (0x8086) | 1.2 | VDV1DP25 | 32 |
| `Micron_3500_MTFDKBA512TGD` | Micron (0x1344) | 2.0 | P8MA002 | 33 |
| `KXG60ZNV1T02 TOSHIBA` | Toshiba (0x1179) | 1.3 | AGGA4104 | 33 |
| `SAMSUNG MZVL2512HCJQ-00B00` | Samsung (0x144d) | 1.3 | GXA7801Q | 33 |
| `SAMSUNG MZVL2512HCJQ-00B07` | Samsung (0x144d) | 1.3 | GXA7802Q | 33 |
| `SAMSUNG MZQLB960HAJR-00007` | Samsung (0x144d) | 1.2 | EDA5502Q | 34 |
| `SAMSUNG MZVLB1T0HBLR-00000` | Samsung (0x144d) | 1.3 | EXF7201Q | 34 |

Two distinct failure modes hide behind that single column, and the exporter
handles them differently:

- **Refusal.** The controller answers page 0xC0 with "Invalid Log Page"
  (SCT=1, SC=0x09). Each refusal appends an entry to the drive's own error
  log, so the answer is asked for once per process and remembered.
- **Foreign data.** The Intel and Dell P4510 answer 0xC0 with 512 bytes
  whose GUID is not the OCP one. Nothing is appended to the error log, and
  the page is rejected at the GUID check.

## Model name is not enough

The Intel and Dell P4510 entries above are the same silicon. The
Dell-branded firmware reports `NN=1` and no Namespace Management support,
while the Intel one reports `NN=128` and supports it. OEM firmware changes
what a controller exposes; do not infer capability from a model number.

## Known gaps

- **OCP layout is read up to v3.** Versions 2 and 3 were verified
  byte-identical here. OCP extends the page additively into reserved
  space, so a v5 drive parses correctly for every field the exporter
  reads — but the two fields v5 adds at offsets 192-207, PCIe retraining
  count and power state change count, are not read.
- **Multipath and multi-namespace subsystems are untested.** Every device
  measured has exactly one namespace.

## Adding your drive

Run `nvme_logpage_exporter dump --out ./dump` and open an issue with the
result. Serials are scrubbed by default, from the Identify serial field,
from SUBNQN, and by a scan of every buffer written. Attach your own
`nvme smart-log` and `nvme id-ctrl` output as ground truth; the dump then
becomes a fixture with expected values, and the model joins a table that
regression tests keep honest. See [`CONTRIBUTING.md`](CONTRIBUTING.md).
