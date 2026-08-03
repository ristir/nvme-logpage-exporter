# nvme_logpage_exporter

A Prometheus exporter for NVMe drives that reads log pages **directly via
ioctl**. No `smartctl`, no `nvme-cli`, no external process spawned per
scrape.

## Why another one

Every existing NVMe exporter is a wrapper around an external tool:
`smartctl_exporter` parses `smartctl` output, and `nvme_metrics.py`,
`fritchie/nvme_exporter` and `yongseokoh/nvme_exporter` all shell out to
`nvme-cli`. None of them talks to the device itself.

That indirection has three consequences this project exists to avoid:

- **Coverage.** `nvme-cli` is absent on some production hosts, `smartctl`
  is absent on others. A wrapper around either tool has blind spots that
  depend on which package happened to get installed. Talking to the
  device through the kernel's own NVMe ioctl interface has none.
- **Accuracy.** `nvme_metrics.py`, for example, names a metric
  `nvme_controller_busy_time_seconds_total` but writes the raw field into
  it unconverted, even though the specification defines that field in
  minutes — a 60x error. See
  `prometheus-community/node-exporter-textfile-collector-scripts/nvme_metrics.py`:
  `metrics["controller_busy_time"].labels(device_name).inc(int(smart_log["controller_busy_time"]))`,
  no conversion applied before the `_seconds_total` metric is incremented.
  Power-on time is specified in hours, data volumes in "data units" of
  512,000 bytes, and temperature in Kelvin; getting any of these
  conversions wrong silently corrupts the number without ever producing
  an error.
- **Alertability.** Existing exporters expose `critical_warning` as a raw
  bitfield. PromQL has no bitwise operators, so an alert on a specific
  condition either doesn't get written or ends up as something like
  `floor(nvme_critical_warning / 4) % 2 == 1`. Here, the field is
  decomposed into one flag metric per condition instead.

On top of that, the warning and critical temperature thresholds are read
from the controller itself (WCTEMP and CCTEMP from Identify Controller),
not hardcoded. That turned out to matter: across the hardware this
exporter has been checked against, the measured warning threshold spans
roughly **17 degrees** — from about 70°C on some controllers up to about
87°C on another. A single hardcoded threshold would be wrong for most of
that range in one direction or the other.

## What it exports today

- **Health log (page 0x02):** critical warning, decomposed into one
  `nvme_logpage_critical_warning_flag{flag=...}` series per condition instead
  of a raw bitfield; temperature (composite and, where present, individual
  sensors); available spare and its threshold; percentage used;
  data units read/written converted to bytes; host read/write commands;
  controller busy time; power cycles and power-on time; unsafe shutdowns;
  media errors; error log entry count; and time spent above the warning
  and critical temperature thresholds.
- **Controller state**, from `/sys/class/nvme/nvmeN/state`, as two
  metrics that answer two different questions:
  `nvme_logpage_controller_state{state}` carries the kernel's own string
  for diagnosis, and `nvme_logpage_controller_live` is a plain 1/0 to
  alert on. Worth having because a controller that is not live refuses
  admin commands: without it, a drive stuck in `resetting` produces a wall
  of read timeouts that look like a defect in the exporter. See
  "Controller state" below for why it is two metrics and not one.
- **Identify Controller:** model, firmware, vendor ID, NVMe version,
  total capacity, maximum and active namespace counts, and the WCTEMP/
  CCTEMP temperature thresholds described above.
- **sysfs:** namespace size and logical block size, and namespace
  membership in a Linux `md` software RAID array, so the namespace
  metrics can be joined against `node_disk_*` and `node_md_*` from
  `node_exporter`.
- **Self-diagnostics:** `nvme_logpage_scrape_success`,
  `nvme_logpage_scrape_duration_seconds`,
  `nvme_logpage_errors_total{reason}`,
  `nvme_logpage_supported{page}`, `nvme_logpage_devices_discovered`, and
  the standard `nvme_logpage_build_info`.
- **OCP extended health log (page 0xC0), on drives that have it:**
  `nvme_logpage_media_written_bytes_total`, `nvme_logpage_media_read_bytes_total`,
  `nvme_logpage_bad_nand_blocks_total{area}` and
  `nvme_logpage_bad_nand_blocks_normalized{area}` (`area` is `user` or
  `system`), `nvme_logpage_xor_recovery_total`,
  `nvme_logpage_uncorrectable_read_errors_total`,
  `nvme_logpage_soft_ecc_errors_total`, `nvme_logpage_e2e_errors_detected_total`,
  `nvme_logpage_e2e_errors_corrected_total`, `nvme_logpage_system_area_used_ratio`,
  `nvme_logpage_refresh_count_total`, `nvme_logpage_user_erase_cycles_max`,
  `nvme_logpage_user_erase_cycles_min`, `nvme_logpage_thermal_throttle_events_total`,
  `nvme_logpage_thermal_throttle_ratio`, `nvme_logpage_pcie_correctable_errors_total`,
  `nvme_logpage_incomplete_shutdowns_total`, `nvme_logpage_free_blocks_ratio`,
  `nvme_logpage_capacitor_health`, `nvme_logpage_unaligned_io_total`,
  `nvme_logpage_security_version`, `nvme_logpage_namespace_used_bytes`,
  `nvme_logpage_plp_starts_total`, `nvme_logpage_endurance_estimate_bytes`,
  and `nvme_logpage_ocp_info{version}`. See "OCP extended health log" below.
- **Firmware Slot Information (page 0x03):**
  `nvme_logpage_firmware_slot_info{slot,revision}` (one series per populated
  slot), `nvme_logpage_firmware_active_slot`, and `nvme_logpage_firmware_next_slot`
  (emitted only while an activation is pending).
- **Error Information (page 0x01):**
  `nvme_logpage_error_log_retained_entries{status_code_type,status_code}` —
  every entry the log retains, grouped by status. Its length is ELPE+1 from
  Identify Controller, 64 to 256 entries on the hardware surveyed, and the
  read is sized to match. A fixed 512-byte read would have covered 8: one
  drive here fills all 64. Diagnostic only: the log survives resets and
  carries no timestamp, and it counts admin commands the drive rejected as
  unimplemented, including probes from other tools on the same host.

This has been checked against real hardware: 12 models from 5 vendors,
28 devices across 11 hosts. On a Samsung PM9A1, all 20 comparable metrics
matched a simultaneous `smartctl` sample exactly. Four of the 12 models
serve the OCP log and implement every field it defines; the rest serve the
three standard pages and export 32-34 metrics instead of 57.

**[`HARDWARE.md`](HARDWARE.md) lists every model measured**, what each one
serves, and the quirks worth knowing before you trust a number.

## Controller state

The kernel's view of a controller is exported twice, deliberately.

`nvme_logpage_controller_state{state="live"}` passes the kernel's string
through as a label, whatever it is. The set of states is a driver-internal
enum, not something the NVMe specification defines, and the attribute is
documented under `Documentation/ABI/testing` — which by kernel convention
means it may change, and it has: members have been added, renamed and
removed across releases. Enumerating a fixed list of states as 0/1 series
would mean a state introduced by a future kernel arrives as an all-zero
set with the real value nowhere to be found.

`nvme_logpage_controller_live` is a plain 1/0 on the same fact, and it is
the one to alert on. Putting the state in a label makes the labelled
series unsuitable as an alerting target for two reasons:

- The series is replaced, not updated, when the state changes. The old one
  lingers for the staleness window, so for those minutes a recovered
  controller still has a non-live series and any count of unhealthy
  controllers reads high.
- `for:` requires the same series to persist. A controller flapping
  between two unhealthy states changes series identity on each hop and
  resets the timer, so the sickest drives would be the ones that never
  fire.

Neither metric is emitted when the attribute is unreadable. "Unknown" must
not be reported as "not live" — that pages someone over a missing sysfs
file rather than over a sick drive.

`node_exporter` reports the same attribute as `node_nvme_info{state}`,
which is its only NVMe metric — no health data of any kind. There is no
name collision with this exporter; its prefix is `node_`.

## OCP extended health log (page 0xC0)

Page 0xC0 is the OCP Datacenter NVMe SSD Specification's SMART / Health
Information Extended log.

- **Which drives have it.** The split is by drive class, not by vendor:
  datacenter models expose it, client models from the same vendor do not.
  Confirmed present on KIOXIA KCD8XRUG1T92, Micron MTFDKCC960TGP, Samsung
  MZQL27T6HBLA and Micron 7450 MTFDKCC960TFR; confirmed absent on the
  client-class Micron and Samsung drives checked so far.
- **Why the GUID is checked.** Intel SSDPE2KX010T8 and Dell Express Flash
  P4510 both answer a read of page 0xC0 — with data of their own and an
  all-zero GUID at offset 496. Decoded by the OCP layout, those bytes
  produce plausible-looking wear figures anyway. The GUID
  (`AFD514C9-7C6F-4F9C-A4F2-BFEA2810AFC5`) is the only thing that
  distinguishes a real OCP page from this case; the exporter treats a
  mismatch the same as "page absent".
- **Absent fields.** Any field may be reported as all ones, meaning "not
  implemented". Such fields are omitted from the scrape rather than
  exported as zero. A KIOXIA KCD8XRUG1T92 does this for Bad System NAND
  Blocks.
- **Write amplification.** `Data Units Written` from page 0x02
  (`nvme_logpage_written_bytes_total`) is a host-side counter: dividing it by
  itself gives 1.00 on every drive. True write amplification needs page
  0xC0's physical media counter instead:

  ```promql
  rate(nvme_logpage_media_written_bytes_total[1h])
  / rate(nvme_logpage_written_bytes_total[1h])
  ```

  Measured on live drives: Samsung MZQL27T6HBLA 1.145, KIOXIA
  KCD8XRUG1T92 2.929.
- **`nvme_logpage_capacitor_health` is not a percentage**, despite the OCP
  specification calling it one — real drives report 162 and 231. It is
  exported unscaled, on a vendor-defined scale.
- **`nvme_logpage_namespace_used_bytes` carries a `namespace` label**, always
  namespace 1, which is what OCP's NUSE field is defined for. That label set
  is identical to `nvme_logpage_namespace_size_bytes`'s, so the two divide
  directly:

  ```promql
  nvme_logpage_namespace_used_bytes / nvme_logpage_namespace_size_bytes
  ```

  Skipped entirely on a controller that exposes no namespace 1.
- **`nvme_logpage_endurance_estimate_bytes` is not a warranty figure.** The
  log field is in units of 10^9 bytes and the exporter scales it to
  bytes. What a drive reports there, however, varies by vendor: a Micron
  7450 MTFDKCC960TFR returns exactly its rated 1 DWPD over five years,
  while a KIOXIA KCD8XRUG1T92 returns 5.3x its rating and a Samsung
  MZQL27T6HBLA 4.1x — those appear to quote raw media capability rather
  than warranted endurance. Compare a drive against itself over time,
  not against its datasheet or against another model.
- **`nvme_logpage_thermal_throttle_events_total` saturates.** The field is
  one byte wide. At 255 it stops counting rather than wrapping to 0.
- **Why page 0xC0 is polled only once per controller.** A controller
  appends an entry to its own Error Information log every time it
  refuses a log page read — measured at exactly one entry per refused
  read, on a Samsung MZQLB1T9HAJR. Polling an absent page on every scrape
  would therefore inflate `nvme_logpage_error_log_entries_total` on every
  drive without OCP, forever. The exporter caches "page not supported"
  per controller for exactly this reason. This is a correctness
  requirement, not an optimization: do not "simplify" it away by
  deleting the cache. The cache is only populated when the refusal is
  recognised as "page unsupported"; a refusal with any other status code
  is an ordinary ioctl error, deliberately left uncached, so it would be
  re-probed every scrape — but that path is loud, not silent: it also
  raises `nvme_logpage_errors_total{reason="ioctl"}` and drops
  `nvme_logpage_scrape_success` to 0 on every such scrape.
- **`nvme_logpage_error_log_entries_total` is not a health signal on its
  own.** The counter (page 0x02) includes admin commands the drive
  rejected because it does not implement them — including probes issued
  by other monitoring tools polling the same controller.

## Privileges

Two independent conditions have to be met. Both have now been verified
on real hardware, and the failure for each is reported with a distinct
message rather than a generic "permission denied": opening the device
file fails with `failed to open device file`, and a rejected admin
command fails with `missing CAP_SYS_ADMIN` — the second one is reached
only once the first has already succeeded, which is exactly what "two
independent conditions" means in practice.

1. **`CAP_SYS_ADMIN`** — the kernel requires it on `NVME_IOCTL_ADMIN_CMD`
   regardless of who owns the process.
2. **Access to the `/dev/nvmeX` controller device.** This is the
   character device the admin commands above go through — not the
   `/dev/nvmeXn1` namespace block device, which is a different node with
   its own, more permissive default permissions. On a stock Ubuntu
   24.04 host the controller device is `0600 root:root`, and the stock
   `disk` group grants nothing there (it owns the namespace block
   devices, not the controller). The udev rule in
   `packaging/udev/99-nvme-logpage-exporter.rules` is what changes that
   device's group and mode, to a dedicated `nvme_logpage` group rather than
   `disk` — reusing `disk` would also grant read/write on every
   namespace block device on the host. Installing the rule is
   **required** for any non-root deployment, not an alternative to group
   membership.

### Under systemd: verified non-root

The provided unit (`packaging/systemd/nvme_logpage_exporter.service`) has
been measured running as an unprivileged, non-root user and successfully
reading the health log: `AmbientCapabilities=CAP_SYS_ADMIN` is what makes
that possible, because it is the one mechanism that carries a capability
into a non-root process's effective set at exec time. Combined with the
udev rule for device access, no root and no `sudo` are needed.

Measured on two Ubuntu 24.04 hosts (systemd 255), one carrying drives
with an OCP page and one without. The running process holds exactly one
capability and nothing else:

```
$ grep ^Cap /proc/$(systemctl show -p MainPID --value nvme_logpage_exporter)/status
CapPrm: 0000000000200000
CapEff: 0000000000200000
CapBnd: 0000000000200000
CapAmb: 0000000000200000
```

Bit 21 is `CAP_SYS_ADMIN`; the bounding set contains that and nothing
more, so the process cannot acquire any other capability even if it
tried.

The two access conditions are independent, and the exporter's own
metrics tell them apart. With the unit installed but the udev rule
missing, the service starts and every device fails with

```
nvme_logpage_errors_total{device="nvme0",reason="open"} 5
```

`reason="open"` means the device file could not be opened — install the
udev rule. `reason="capability"` would mean the opposite: the file
opened but the ioctl was refused, which points at
`AmbientCapabilities`, not at udev.

### In a container: root inside the container, not `--privileged`

The container path was measured separately, and it does **not** work the
same way: Docker's `--cap-add=SYS_ADMIN` only populates the *bounding*
set for the container's process. It reaches the *effective* set solely
for uid 0 — Docker has no ambient-capability flag equivalent to
`AmbientCapabilities`. A container running as a non-root user therefore
ends up with `CAP_SYS_ADMIN` in its bounding set but not its effective
set, and every device read fails.

So the shipped image runs as root **inside the container** (see the
comment in `Dockerfile`). This is still not `--privileged`: running with
`--cap-add=SYS_ADMIN --device=/dev/nvme0 -v /sys:/sys:ro` and no
`--privileged` is enough, and was confirmed to produce the full metric
set. `--privileged` would additionally grant every other capability and
access to every device on the host; this setup grants neither.

## Installation

```bash
sudo groupadd --system nvme_logpage
sudo useradd --system --no-create-home --shell /usr/sbin/nologin -g nvme_logpage nvme_logpage

make build
sudo cp bin/nvme_logpage_exporter /usr/local/bin/
# or: sudo make install   (does the same three copies + udev reload below)

sudo cp packaging/systemd/nvme_logpage_exporter.service /etc/systemd/system/
sudo cp packaging/udev/99-nvme-logpage-exporter.rules /etc/udev/rules.d/

sudo udevadm control --reload
sudo udevadm trigger

sudo systemctl enable --now nvme_logpage_exporter
```

`udevadm trigger` is not optional on a first install: the rule only
applies to devices enumerated after it is loaded, so without it the
already-present `/dev/nvmeX` nodes keep their default `0600 root:root`
mode and the first start fails at `open`.

## Running it

```bash
nvme_logpage_exporter --web.listen-address=:9683
```

A ready-made unit is provided at
`packaging/systemd/nvme_logpage_exporter.service`. Non-root operation under
that unit requires the udev rule in `packaging/udev/` to be installed
too — see Privileges and Installation above.

### Flags

| Flag | Default | Meaning |
|---|---|---|
| `--web.listen-address` | `:9683` | Repeatable. Address(es) to listen on. |
| `--web.telemetry-path` | `/metrics` | Path under which metrics are served. |
| `--web.config.file` | (none) | TLS and basic auth config file, provided by `exporter-toolkit`. Without it the server has neither. See the [web-configuration docs](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md). |
| `--web.systemd-socket` | `false` | Use systemd socket activation listeners instead of `--web.listen-address` port listeners. Linux only; registered unconditionally by `exporter-toolkit` on that platform. |
| `--nvme.source` | `auto` | `auto` (ioctl) or `dir:<path>` to replay dumps. |
| `--nvme.timeout` | `5s` | Timeout for polling a single device. |
| `--log.level` | `info` | One of `debug`, `info`, `warn`, `error`. |
| `--log.format` | `logfmt` | One of `logfmt`, `json`. |

The `dump` subcommand additionally takes `--out` (required, output
directory) and `--keep-serial` (see Working with dumps below).

### Container

```bash
make docker
```

builds a `scratch`-based image containing only the statically linked
binary and no entrypoint script. It is `linux/amd64` regardless of the
host you build on. Released images are multi-arch, `linux/amd64` and
`linux/arm64`, and published on a tag to
`ghcr.io/ristir/nvme-logpage-exporter` and Docker Hub. Run it with the
devices and `/sys` mounted in from the outside, `--cap-add=SYS_ADMIN`,
and no `--privileged`:

```bash
docker run --cap-add=SYS_ADMIN \
  --device=/dev/nvme0 --device=/dev/nvme1 \
  -v /sys:/sys:ro -p 9683:9683 \
  nvme_logpage_exporter:<version>
```

Kubernetes is the exception, and it needs `privileged: true`. There is no
field for a device cgroup rule, so a hostPath `/dev` gets `EPERM` on open
however many capabilities the pod is given — measured, not assumed. The
namespace also needs `pod-security.kubernetes.io/enforce: privileged`. A
ready DaemonSet is in [`packaging/kubernetes/`](packaging/kubernetes/).

## Working with dumps

The exporter can read from a directory of previously captured dumps
instead of a live device — useful for investigating bug reports and for
development without NVMe hardware on hand:

```bash
nvme_logpage_exporter dump --out ./dumps          # capture
nvme_logpage_exporter --nvme.source=dir:./dumps   # replay
```

Serial numbers are scrubbed by default: `meta.json`'s serial field is
replaced, `identify.bin`'s SN field (bytes 4:23) and SUBNQN field (bytes
768:1023) are both blanked — SUBNQN commonly embeds the serial verbatim in
the subsystem NQN string — and every captured buffer, including vendor log
pages such as 0xC0, is then scanned for any remaining verbatim occurrence
of the serial. Check the output before sharing a dump anyway: this is
defense in depth, not a guarantee that every vendor-specific encoding of
the serial has been anticipated. See `CONTRIBUTING.md` for the full dump
workflow, including how to send one for a drive this project doesn't yet
have a parser for.

## Alerting rules

`packaging/alerts/nvme-logpage-exporter.rules.yml` is a starting set of
thirteen rules for Prometheus or vmalert. Read it before deploying it —
each rule carries its reasoning, and several thresholds are deliberately
not the ones other NVMe alerting examples use.

Three choices in there are worth surfacing here, because they are the ones
most likely to be "corrected" by someone skimming:

**Temperature and spare capacity are compared against the drive's own
thresholds**, never against a constant. The controller publishes both, and
across the fleet this exporter was built against they span 17 degrees —
from 69.9 C on Intel and Dell units to 86.9 C on some Samsung parts. A
fixed line at 70 C sits on top of one model's normal operating point and
far above another's.

**Wear is alerted on by rate, not only by level.** A drive at 60% that is
not moving is less urgent than one at 20% climbing fast. There is also a
rule that fires per *host* when two or more drives are projected to wear
out within six months of each other: drives bought together and given
identical work reach the end together, and an array whose members all fail
in the same week is an outage rather than a replacement.

**Counter deltas use `max_over_time` minus `min_over_time`, not
`increase()`.** These counters come from the drive and carry its entire
lifetime, so where the exporter has been running for less than the alert
window, `increase()` reads the counter's first appearance as a rise from
zero and reports years of history as if it had just happened. Measured on
a fresh deployment: `increase(nvme_logpage_unsafe_shutdowns_total[24h])`
returned exactly the lifetime value on every affected drive. For a
critical-severity rule that means paging on every new host.

**Controller state is alerted on twice, on purpose.** `dead` is critical —
the driver has given up and nothing further happens without intervention —
while every other unhealthy state is a warning, since most of them are
recoverable. The critical rule matches the state label directly, which is
safe only because a controller never leaves `dead` on its own; the warning
rule uses the boolean, because states that *do* change would otherwise
reset its `for` timer on every hop. A dead controller therefore raises
both. Suppress the warning with an Alertmanager inhibit rule rather than
weakening either — the snippet is in the header of the rules file.

Two metrics are deliberately left un-alerted. `nvme_logpage_error_log_entries_total`
counts admin commands the drive rejected, including ones rejected only
because it does not implement them — any monitoring tool on the host that
probes an optional log page adds to it, so alerting on it produces noise
proportional to how many tools you run. Write amplification is a capacity
planning input, not a fault.

## What it doesn't do

- **No per-namespace I/O statistics.** NVMe log pages don't carry them;
  the kernel already exposes that data via `node_exporter`
  (`node_disk_*`). Duplicating that collector wouldn't add anything —
  the namespace metrics here exist to be joined against it, not to
  replace it.
- **No network calls.** Nothing is fetched from a URL at any point.
- **NVMe only.** No SATA, no SCSI.
- **OCP page layout up to v3.** Offsets are identical across the versions
  verified here, so newer pages still parse; the two fields v5 adds are
  not read. See [`HARDWARE.md`](HARDWARE.md).

Don't run this alongside another NVMe exporter on the same host — they
will both poll the same log pages independently.

## License

Apache License 2.0. See [`LICENSE`](LICENSE).
