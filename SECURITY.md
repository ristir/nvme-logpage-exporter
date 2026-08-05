# Security

This exporter needs `CAP_SYS_ADMIN`. That is the broadest capability Linux
has, so this document states exactly why it is required, what the process
can do with it, what each deployment grants beyond it, and what a
compromise would cost.

## Reporting a vulnerability

Open a private security advisory through GitHub's *Report a vulnerability*
button on the Security tab. Please do not open a public issue first.

Fixes go to the latest minor release. Older ones are not backported.

## Why the capability is required

Reading an NVMe log page means issuing `NVME_IOCTL_ADMIN_CMD` against the
controller's character device. The kernel gates that ioctl, and there is no
narrower capability for it.

Since Linux 6.2 the check is finer than a bare `capable(CAP_SYS_ADMIN)`:
[`nvme_cmd_allowed()`](https://github.com/torvalds/linux/blob/v6.2/drivers/nvme/host/ioctl.c)
lets an unprivileged caller issue a small allowlist of Identify commands. It
arrived in
[855b7717f44b](https://github.com/torvalds/linux/commit/855b7717f44b13e0990aa5ad36bbf9aa35051516),
"nvme: fine-granular CAP_SYS_ADMIN for nvme io commands", and is absent from
v6.1. Get Log Page is not on that allowlist, and the log pages are what this
exporter reads.

Measured with `--cap-drop=ALL` and nothing added:

| Kernel | Identify Controller | Get Log Page | Metrics exported |
|---|---|---|---|
| 7.0, 6.8 | works | `EACCES` | 28 of 136 |
| 5.15 | `EACCES` | `EACCES` | 0 |

The 28 are `device_info` plus scrape-level series; no drive health figures
among them. Totals here and below are per host and scale with the drive
count: 136 on the host in this table, 134 on the two-drive host used for the
systemd measurements.

**With the capability granted, every one of those kernels works normally.**
The table describes what is lost by withholding it, not a compatibility
matrix. Verified on 4.15 and newer; older kernels have not been tried.

Dropping the capability after opening the device files does not help
either: the kernel checks on every ioctl, not at `open`.

## What the process can do with it

### In the shipped binary

- **No subprocesses.** `os/exec` is not imported anywhere in the tree. There
  is no `smartctl` to substitute, no argument to inject, no `PATH` to
  influence.
- **The device is opened read-only** — `os.O_RDONLY`.
- **Two opcodes exist in the code**, both read-only: Get Log Page and
  Identify. There is no write, `Format NVM`, `Sanitize` or
  `Firmware Download` path to reach — those commands are absent, not
  guarded by a check.
- **Log page IDs are compile-time constants**: `0x01`, `0x02`, `0x03`,
  `0xC0`. Nothing selects them at runtime.
- **No request-controlled value selects a device, an opcode or a log page
  ID.** Those are compile-time constants. Path, query, body and all but one
  header are unused. The one value that does cross is
  `X-Prometheus-Scrape-Timeout-Seconds`: it is parsed as a duration,
  rejected if unparseable, NaN or non-positive, capped at one hour, and
  then reaches the ioctl only as the command's timeout field — floored at
  1 ms and bounded above by `--nvme.timeout`, which wraps every per-device
  context. A scrape can shorten its own deadline; it cannot lengthen it,
  redirect it, or change what is read.
- **The image is `FROM scratch`**: one static binary, no shell, no package
  manager, no utilities.

The attack surface is one HTTP endpoint serving text, plus the bytes the
drive returns. The latter is parsed at fixed offsets into a
pre-sized buffer with Go's bounds checking, and the parsers are fuzzed in
CI. A hostile drive can produce wrong numbers; it does not get to reach
past the buffer.

### After a compromise

The list above describes the binary as shipped, not the ceiling on what the
process may do. An attacker able to execute code in it builds the command
structure directly: the kernel does not filter on the NVMe opcode, so
`Format NVM`, `Sanitize` and `Firmware Commit` are reachable through the same
ioctl that reads a log page, on the same read-only descriptor. Absent opcodes
in the source raise the bar for reaching that point; they do not bound it.

What still holds comes from the deployment and the kernel, not from the
source, and so differs between the three. Under the systemd unit the process
keeps one capability in the bounding set, gains no new privileges, can neither
create nor join a namespace, and reaches only the NVMe controller nodes.
Docker and Kubernetes draw those boundaries differently; see What each
deployment grants.

## What the endpoint discloses

This is a risk in normal operation, not only after a compromise.

Every series carries `device` and `serial`; `nvme_logpage_device_info` adds
the model and the firmware revision, and `nvme_logpage_firmware_slot_info`
reports the revision in each slot. Taken together, `/metrics` is a hardware
inventory: which drive models are deployed, their serial numbers, and which
firmware each one runs. It therefore locates every drive running a firmware
revision with a published vulnerability.

The exposure does not end at the endpoint. Every label is stored in the
monitoring backend and, wherever `remote_write` is configured, forwarded on
— including to third-party SaaS. Serial numbers and firmware revisions leave
the perimeter with the metrics unless they are dropped by relabelling first.

**The endpoint is unauthenticated by default**, and on a host it listens on
every interface unless told otherwise.

Two ways to close it:

- **TLS and basic auth.** The exporter uses `exporter-toolkit`, so
  `--web.config.file` takes the standard
  [web configuration](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md)
  with `tls_server_config` and `basic_auth_users`.
- **Bind to an internal address.** `--web.listen-address` is repeatable and
  accepts an address, not just a port: `--web.listen-address=10.0.0.5:10192`
  keeps the endpoint off public interfaces.

## What each deployment grants

### systemd — least privilege

```ini
AmbientCapabilities=CAP_SYS_ADMIN
CapabilityBoundingSet=CAP_SYS_ADMIN
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ProtectKernelTunables=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6
RestrictNamespaces=true
LockPersonality=true
MemoryDenyWriteExecute=true
RestrictSUIDSGID=true
RestrictRealtime=true
ProtectKernelModules=true
PrivateIPC=true
RemoveIPC=true
ProtectKernelLogs=true
ProtectHostname=true
ProtectClock=true
ProtectProc=invisible
UMask=0077
SystemCallFilter=@system-service
SystemCallArchitectures=native
DevicePolicy=closed
DeviceAllow=char-nvme r
```

The unit runs as a dedicated unprivileged user. A udev rule grants that
user's group read access to the controller nodes only, not through the `disk`
group that owns every block device.

Metric counts below come from a two-drive host, where this unit exports 134
series.

| Directive | Effect | Metrics |
|---|---|---|
| `AmbientCapabilities=CAP_SYS_ADMIN` | Permits `NVME_IOCTL_ADMIN_CMD`; nothing else in the unit substitutes for it | Full set. Without it, no log page is readable: scrape-level series only, no drive health |
| `CapabilityBoundingSet=CAP_SYS_ADMIN` | No second capability can be acquired, including through a setuid binary | Unchanged |
| `NoNewPrivileges=true` | Blocks privilege gain across `execve` | Unchanged |
| `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp` | The whole filesystem hierarchy is mounted read-only except what `ReadWritePaths=` names; `/home` is inaccessible and `/tmp` is private to the service | Unchanged for serving. `dump --out` needs a path listed in `ReadWritePaths=` |
| `ProtectKernelTunables`, `ProtectControlGroups` | `/proc/sys` and the cgroup tree become read-only | Unchanged |
| `RestrictAddressFamilies=AF_INET AF_INET6` | No unix sockets, so no credential passing | Unchanged |
| `RestrictNamespaces=true` | `unshare` and `setns` refused — the usual escalation route out of this capability | Unchanged |
| `LockPersonality`, `MemoryDenyWriteExecute` | No `personality()` switch, no writable-executable mappings | Unchanged |
| `RestrictSUIDSGID`, `RestrictRealtime`, `ProtectKernelModules` | No setuid bits created, no realtime scheduling, no module loading or reading | Unchanged |
| `PrivateIPC`, `RemoveIPC` | Private SysV IPC namespace, and its objects go when the service stops | Unchanged |
| `ProtectKernelLogs`, `ProtectHostname`, `ProtectClock` | No `dmesg` ring, no hostname change, clock devices read-only | Unchanged |
| `ProtectProc=invisible`, `UMask=0077` | Other users' processes hidden in `/proc`; files created private | Unchanged |
| `SystemCallFilter=@system-service` | Denies `mount`, `swapon`, `ptrace`, `bpf`, `perf_event_open`, `quotactl` and the rest of the groups outside that set. Floor of systemd 249: on 237 the set lacks `arch_prctl` and `sched_getaffinity` and the Go runtime takes `SIGSYS` | Unchanged on 249 and newer. On 237 the process does not start, so none |
| `SystemCallArchitectures=native` | No secondary-architecture entry into the filter | Unchanged |
| `DevicePolicy=closed` + `DeviceAllow=char-nvme r` | The capability reaches the NVMe controller nodes and nothing else — `/dev/mem`, `/dev/kmsg` and the block nodes return `EPERM` on open. The major is dynamic, so the rule names the driver and systemd resolves it through `/proc/devices`. Read suffices: the cgroup gates `open()`, and the passthrough runs on a read-only descriptor. `rw` only adds opening the node for writing, which nothing here does. `ProtectClock` adds `char-rtc r` to the same list | Full set. With the policy closed and no allow rule, 62 of 134: `scrape_success` reads 0, `errors_total` carries `reason="open"`, and every drive metric is absent |

`systemd-analyze security` rates this unit 2.0. What it still flags is
inherent: the ambient capability, access to device nodes, and a listening
socket.

Two more directives, deliberately left out:

| Directive | Effect | Metrics |
|---|---|---|
| `ProcSubset=pid` | Hides everything in `/proc` outside the process directories, including `/proc/stat`, which the client library reads to place process start time on the wall clock | 133 of 134: `process_start_time_seconds` disappears |
| `PrivateDevices=` | Substitutes `/dev` and hides the controller nodes. Do not set | Same loss as a closed policy with no allow rule |

No directive restricts which NVMe command is sent. seccomp filters on the
ioctl request number, and `NVME_IOCTL_ADMIN_CMD` is the one in use; the opcode
sits in a structure behind the pointer, out of a filter's reach.

This is the recommended way to run on bare metal.

### Docker

```bash
docker run -d --name nvme_logpage_exporter --restart=unless-stopped \
  --user 0:0 \
  --cap-drop=ALL --cap-add=SYS_ADMIN \
  --security-opt=no-new-privileges \
  --read-only \
  --device=/dev/nvme0 --device=/dev/nvme1 \
  -v /sys:/sys:ro \
  -p 10192:10192 \
  ristir/nvme-logpage-exporter:latest
```

Without `--cap-drop=ALL` the container also carries Docker's
fourteen default capabilities — `chown`, `dac_override`, `setuid`, `mknod`,
`net_raw` and the rest — none of which this exporter uses. With it,
`CapEff` is `0000000000200000`, a single bit, and the metric count is
unchanged.

`--user 0:0` cannot be dropped. Docker places an added capability in the
bounding set only; it reaches the effective set for uid 0 alone, because
Docker has no ambient-capability option. A non-root user inside the
container ends up with an empty `CapEff` and every read fails.

`--privileged` is **not** required here.

### Kubernetes

The DaemonSet runs `privileged: true`, which is broader than
`CAP_SYS_ADMIN` alone: it grants the full capability set, disables seccomp
and detaches the AppArmor profile. A compromise of that pod carries what is
needed to take over the node.

`--device` under Docker writes a device cgroup rule. Kubernetes has no field
that writes one, and a hostPath `/dev` without it returns `EPERM` on open.

A device plugin writes that rule through the kubelet, which is what would
allow `privileged: true` to be replaced by `capabilities: {drop: [ALL], add:
[SYS_ADMIN]}`. Note that the runtime's default seccomp profile allows
`mount`, `setns` and `unshare` whenever `CAP_SYS_ADMIN` is present, so
recovering it narrows less than it appears; the AppArmor profile and the
smaller capability set are where the difference is.

**Prefer the systemd unit on bare metal.** Use the DaemonSet where nodes are
managed only through Kubernetes, and weigh the wider grant against that
convenience.

## Blast radius

Reaching any of the above requires code execution in the process. What that
would then be worth:

| Deployment | Consequence of compromise | seccomp |
|---|---|---|
| systemd | one capability, no new privileges, namespaces restricted, read-only filesystem, `/dev` limited to NVMe character devices | `@system-service`, see above |
| Docker, with the flags above | one capability, read-only rootfs, no new privileges, only the listed devices | active, but see below |
| Docker, without them | fifteen capabilities, writable rootfs, privilege escalation through setuid binaries possible | active, but see below |
| Kubernetes DaemonSet | enough to take over the node | disabled by `privileged` |

The default seccomp profile stays in force under Docker, but it does not
narrow `CAP_SYS_ADMIN` itself. One rule in that profile carries
`includes.caps: [CAP_SYS_ADMIN]` and allows `mount`, `umount`, `umount2`,
`setns`, `unshare`, `clone`, `bpf`, `perf_event_open`, `quotactl`,
`fanotify_init`, `sethostname`, `setdomainname`, `syslog` and the newer
mount API — `fsopen`, `fsmount`, `move_mount`, `open_tree`, `mount_setattr`
— *precisely because* the capability is present. Adding `--cap-add=SYS_ADMIN`
is what unlocks them. `pivot_root` appears in no rule and stays blocked by
the profile's `SCMP_ACT_ERRNO` default.

Seccomp here removes unrelated syscalls, not the escalation primitives.

## Known future work

Splitting privileges would remove the capability from the network-facing
process entirely: a small helper holding `CAP_SYS_ADMIN` and speaking only
Get Log Page and Identify over a unix socket, with the HTTP process running
unprivileged. The `Source` interface is already the seam this would use —
`replay.go` demonstrates it by serving the same metrics from a dump. This is
not implemented today.
