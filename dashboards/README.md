# Grafana dashboard

`nvme-logpage-exporter.json` is an importable dashboard for the metrics this
exporter emits. It needs a Prometheus- or VictoriaMetrics-compatible
datasource scraping the exporter, and nothing else.

## Import

1. Grafana -> Dashboards -> New -> Import.
2. Upload `nvme-logpage-exporter.json`.
3. Pick a datasource for the `datasource` variable.

The `job`, `instance`, `device` and `top_n` variables populate from that
datasource.

## Rows

| Row | Contents |
|---|---|
| Fleet health | totals, unhealthy devices, failing scrapes, worst endurance, model breakdown |
| Worst offenders | `topk` and `bottomk` tables bounded by `top_n` |
| Inventory | controllers, firmware slots, namespaces |
| Endurance and wear | wear over time, spare, write rates, write amplification, projected exhaustion |
| Temperature | composite and per-sensor readings against the drive's own thresholds, throttling |
| Reliability | critical warning flags, errors, retired blocks, unsafe shutdowns, error log by status |
| Activity | byte and command rates, controller busy fraction, unaligned I/O |
| Exporter self-diagnostics | scrape success, log page availability, errors by reason, scrape duration, build info |

The first two rows are open and hold scalars and `topk` tables, so their
element count does not grow with the fleet. The rest draw one series per
device and are collapsed.

## Regenerating

```bash
python3 gen/gen_dashboard.py
```

Edit `gen/gen_dashboard.py` rather than the JSON, and commit both.
