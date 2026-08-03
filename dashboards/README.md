# Grafana dashboard

`nvme-logpage-exporter.json` is a single importable dashboard covering every
metric `nvme_logpage_exporter` emits.

It is **generated**, not hand-edited: run `python3 gen/gen_dashboard.py`
after changing `gen/gen_dashboard.py`, and commit both. Thirty-odd panels
that differ only in query, unit and title are not maintainable as raw
JSON — the previous hand-edited version shipped a value mapping copied
onto panels where its polarity was inverted, painting a healthy fleet
solid red for weeks.

## Import

1. Grafana -> Dashboards -> New -> Import.
2. Upload `nvme-logpage-exporter.json` (or paste its contents).
3. Pick a datasource for the `datasource` variable: any Prometheus or
   VictoriaMetrics datasource that scrapes `nvme_logpage_exporter` works.
4. Done. `job`, `instance` and `device` populate from `label_values(...)`
   against that datasource.

## Requirements

A Prometheus- or VictoriaMetrics-compatible datasource scraping
`nvme_logpage_exporter`'s `/metrics` endpoint. Nothing else: every panel
draws on this exporter alone.

## Built for a fleet, not for a host

The constraint that shapes the layout: **nothing above the collapsed rows
may draw one series per device.** The two rows that are open by default
are scalars and `topk` tables, so they render the same number of elements
on five hundred hosts as on one. Per-device detail lives below, collapsed,
and is meant to be reached by selecting a host.

This is not a stylistic preference. A per-device state timeline across a
few dozen devices is already an unreadable smear of labels, and a
per-device time series across several hundred is both illegible and
expensive to query.

- **Fleet health** (open) — devices, hosts, `Unhealthy`, devices failing
  scrape, devices over their own temperature threshold, worst endurance,
  and how many drives expose the OCP page. Plus a model breakdown and a
  table of devices with a critical warning bit set, which is empty on a
  healthy fleet — an empty panel there is the signal.
- **Worst offenders** (open) — four `topk`/`bottomk` tables bounded by the
  `top_n` variable: most worn, least temperature headroom, highest write
  amplification, fastest wearing. Ranking by *headroom* rather than by
  absolute temperature is what makes a mixed fleet comparable; the
  controller-reported warning thresholds span 17 degrees across these
  models, so a single sorted list of temperatures ranks models, not risk.
  Likewise `fastest wearing` answers "what needs replacing first", which
  absolute wear does not: a drive sitting at 50% and not moving matters
  less than one at 10% climbing quickly.
- **Inventory** — one row per controller, plus firmware slots and
  namespaces with the OCP NUSE field.
- **Endurance and wear** — wear over time, spare against its own
  threshold, host and media write rates side by side, lifetime write
  amplification, projected exhaustion.
- **Temperature** — composite temperature drawn against the thresholds the
  drive reports for itself, per-sensor readings, time spent over
  threshold, throttling.
- **Reliability** — critical warning flags (filtered to those actually
  set), media and uncorrectable read errors, retired NAND blocks, unsafe
  shutdowns, error log entries by status.
- **Activity** — byte and command rates, controller busy fraction,
  unaligned I/O.
- **Exporter self-diagnostics** — scrape success, log page availability,
  errors by reason, scrape duration, build info.

## Write amplification

True write amplification needs the OCP extended health log, and it is the
reason that page is read at all:

```promql
nvme_logpage_media_written_bytes_total / nvme_logpage_written_bytes_total
```

`Data Units Written` from the standard health log is a host-side counter,
so dividing it by itself yields 1.00 on every drive — a mistake worth
naming, because it is easy to build a convincing panel out of it. Measured
on live drives: Samsung MZQL27T6HBLA 1.145, KIOXIA KCD8XRUG1T92 2.929.

The query only returns data where
`nvme_logpage_supported{page="0xc0"} == 1`. The split is by drive
class rather than by vendor: datacenter models expose the page, client
models from the same vendor do not.

Read a high ratio on a drive that has written very little with caution.
At low cumulative writes the figure is dominated by the controller's own
background activity, not by the workload.

## Design notes

- **Green means the same thing everywhere.** `1` is the healthy state for
  `scrape_success` and `supported`, and the *failure* state for
  `critical_warning_flag`. They therefore carry different value mappings.
  Reusing one mapping across all three is how the previous version came to
  show a wall of red on a fleet with nothing wrong — which trains an
  operator to ignore red.
- **Multi-metric tables are joined, not merged.** Grafana's `merge`
  appends frames; it does not join them. Six queries with differing label
  sets came out as every device listed once per metric, each row with a
  different column filled in. The fix has two halves: `group_left` in
  PromQL to give every query the same label set, and `label_replace` to
  stamp a `metric` label for `joinByLabels` to name columns by — the
  arithmetic drops `__name__`, so there is nothing else left to name them
  with. `joinByLabels` must then join on *every* remaining label; one left
  out splits a device across several rows again.
- Legends on per-device series are Table/Right with Last and Max. A bottom
  legend with thirty entries squeezes the plot into a sliver.
- Status colours are used only for state panels, never to tell series
  apart.
- No panel uses two y-axes; series of different scale get separate panels.
- Units are set per panel: `percentunit` for 0..1 ratios, `celsius`,
  `bytes`/`Bps`, `s`/`d`.
