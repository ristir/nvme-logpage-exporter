# Contributing

## Requirements

- Go 1.26 (see `go.mod`).
- All code, comments, strings, log messages and documentation are in
  English.

## Building

A fresh clone has nothing installed. Build the binary before running
anything:

```bash
make build && sudo ./bin/nvme_logpage_exporter dump --out ./my-dumps
```

Run these before opening a PR:

```bash
make test   # go test -race ./...
make lint   # golangci-lint, under GOOS=linux
```

## Send us a dump

The single most useful thing you can do is send a dump of the log pages
from a drive we don't own. The project parses binary log pages against the
NVMe specification, so a parser can be written and verified without us
having the hardware.

```bash
sudo nvme_logpage_exporter dump --out ./my-dumps
```

**Serial numbers are scrubbed by default**, in three layers rather than
one field: Identify Controller's SN field (bytes 4:23) is blanked, its
SUBNQN field (bytes 768:1023) is blanked outright — many controllers build
the subsystem NQN by embedding the serial verbatim, so leaving this field
alone leaks the same serial back through a different offset — and every
captured buffer (Identify and every vendor log page) is then scanned for
any remaining verbatim occurrence of the serial, which is replaced too.
That last pass is what would catch a serial embedded in some vendor region
nobody has enumerated yet. Check the output before you send it anyway —
defense in depth is not a guarantee. If you actually need the serial for
some reason, pass `--keep-serial` deliberately.

Attach the resulting directory to an issue, along with the drive's model
and firmware version.

### Dump directory format

```
<dump-dir>/
  meta.json           # controllers, namespaces, md membership
  <controller>/       # e.g. nvme0, one per controller
    identify.bin      # Identify Controller Data Structure, 4096 bytes
    logpage-0xNN.bin  # one file per captured log page, e.g. logpage-0x02.bin
```

A log page file is only present if the controller supports that page; a
missing file means "unsupported", not "empty".

Real-hardware examples of this format are in `testdata/dumps/`; see
`testdata/dumps/README.md` for what each one covers.

## Requirements for parsers

- Buffer length is checked before reading. A parser must never panic: it
  runs in a process holding `CAP_SYS_ADMIN`.
- Every parser ships with a dump under `testdata/` and a golden file.
- A missing field is not emitted as zero — the series is simply not
  created.
- Units are normalized to their base form: seconds, bytes, degrees
  Celsius, fractions in the range 0..1.
