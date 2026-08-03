# nvme_logpage_exporter

A Prometheus exporter for NVMe drives that reads log pages **directly via
ioctl** — no `smartctl`, no `nvme-cli`, no external process per scrape.

Exposes SMART health, controller state, Identify Controller temperature
thresholds, OCP extended health (including true write amplification),
firmware slots and the error log.

**Source, full documentation and alerting rules:**
https://github.com/ristir/nvme-logpage-exporter

## Tags

| Tag | Contents |
|---|---|
| `latest` | Most recent release |
| `vX.Y.Z` | Pinned release, one per entry on the GitHub releases page |

Both are multi-arch: `linux/amd64` and `linux/arm64`. The image is
`FROM scratch` — a single static binary, no shell, no package manager,
under 5 MB.

## Run it

```bash
docker run -d --name nvme_logpage_exporter --restart=unless-stopped \
  --user 0:0 \
  --cap-add=SYS_ADMIN \
  --device=/dev/nvme0 --device=/dev/nvme1 \
  -v /sys:/sys:ro \
  -p 10192:10192 \
  ristir/nvme-logpage-exporter:latest
```

Then scrape `http://<host>:10192/metrics`.

Pass one `--device` per **controller** — `/dev/nvme0`, not `/dev/nvme0n1`.
Namespaces are read from `/sys`, which is why it is mounted.

## Why those flags

`--privileged` is **not** required under Docker. Two things are, and both
are easy to get subtly wrong:

- **`--cap-add=SYS_ADMIN`** — the NVMe admin passthrough ioctl checks
  `CAP_SYS_ADMIN`. Without it every log page read fails with `EACCES`.
- **`--user 0:0`** — Docker puts an added capability in the bounding set
  only, and it reaches the effective set for uid 0 alone. With a non-root
  user inside the container `CapEff` ends up empty and every read fails
  even though `--cap-add` was given. Measured: 24 metrics instead of 57.

Kubernetes is the exception: there it needs `privileged: true` rather than
`SYS_ADMIN` alone. `--device` writes a device cgroup rule, and Kubernetes
has no field for one, so a hostPath `/dev` on its own gets `EPERM` on open.
Privileged pods are rejected by the `baseline` and `restricted` Pod
Security Standards, so the namespace needs
`pod-security.kubernetes.io/enforce: privileged`. A ready-made DaemonSet
ships in the repository under `packaging/kubernetes/`.

## Flags

| Flag | Default | Purpose |
|---|---|---|
| `--web.listen-address` | `:10192` | Listen address. The port is this exporter's allocation in the Prometheus registry. |
| `--web.telemetry-path` | `/metrics` | Metrics path |
| `--nvme.timeout` | `5s` | Per-device deadline |
| `--nvme.source` | `auto` | `auto`, or `dir:<path>` to replay a dump |
| `--web.config.file` | none | TLS and basic auth, via `exporter-toolkit` |
| `--log.level` | `info` | `debug`, `info`, `warn`, `error` |

## Hardware

Verified in production against 29 models from 5 vendors. Any NVMe
controller works — a drive serving fewer log pages simply exports
fewer metrics and reports which ones it served in `nvme_logpage_supported`.

Model coverage and per-model quirks:
https://github.com/ristir/nvme-logpage-exporter/blob/main/HARDWARE.md

## License

Apache-2.0
