#!/usr/bin/env python3
"""Generate dashboards/nvme-logpage-exporter.json.

The dashboard is generated rather than hand-edited because it is large and
highly repetitive: thirty-odd panels that differ only in their query, unit
and title. Hand-maintaining that in JSON is how the previous version ended
up with a value mapping copied onto panels where its polarity was inverted,
painting a healthy fleet solid red.

Design constraint that shapes everything below: this must stay readable on a
fleet of several hundred hosts. Nothing above the first collapsed row may
render one series per device. The top rows are scalars and topk tables, so
the number of elements drawn is constant no matter how large the fleet gets.
Per-device detail lives in collapsed rows and is reached by selecting a host.

Run: python3 dashboards/gen/gen_dashboard.py
"""

import json
import os

OUT = os.path.join(os.path.dirname(__file__), "..", "nvme-logpage-exporter.json")

DS = {"type": "prometheus", "uid": "${datasource}"}

# Label selector shared by every query. Kept in one place so a new variable
# does not have to be threaded through thirty panels by hand.
SEL = 'job=~"$job",instance=~"$instance",device=~"$device"'

_id = [0]


def nid():
    _id[0] += 1
    return _id[0]


def tgt(expr, legend="", instant=False, fmt="time_series", ref="A"):
    return {
        "datasource": DS,
        "editorMode": "code",
        "expr": expr,
        "legendFormat": legend,
        "range": not instant,
        "instant": instant,
        "format": fmt,
        "refId": ref,
    }


def stat(title, expr, desc, unit="none", thresholds=None, w=3, h=4, x=0, y=0,
         color_mode="value", text_size=None):
    steps = thresholds or [{"color": "green", "value": None}]
    p = {
        "id": nid(),
        "type": "stat",
        "title": title,
        "description": desc,
        "datasource": DS,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "targets": [tgt(expr, instant=True)],
        "fieldConfig": {
            "defaults": {
                "unit": unit,
                "mappings": [],
                "thresholds": {"mode": "absolute", "steps": steps},
            },
            "overrides": [],
        },
        "options": {
            "colorMode": color_mode,
            "graphMode": "none",
            "justifyMode": "auto",
            "orientation": "auto",
            "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
            "textMode": "auto",
        },
    }
    if text_size:
        p["options"]["text"] = {"valueSize": text_size}
    return p


def table(title, targets, desc, transformations=None, overrides=None,
          w=12, h=8, x=0, y=0, sort_by=None, sort_desc=True):
    p = {
        "id": nid(),
        "type": "table",
        "title": title,
        "description": desc,
        "datasource": DS,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "targets": targets,
        "transformations": transformations or [],
        "fieldConfig": {
            "defaults": {
                "custom": {"align": "auto", "cellOptions": {"type": "auto"},
                           "filterable": True, "inspect": False},
                "mappings": [],
                "thresholds": {"mode": "absolute",
                               "steps": [{"color": "text", "value": None}]},
            },
            "overrides": overrides or [],
        },
        "options": {"cellHeight": "sm", "footer": {"show": False},
                    "showHeader": True},
    }
    if sort_by:
        p["options"]["sortBy"] = [{"desc": sort_desc, "displayName": sort_by}]
    return p


def timeseries(title, targets, desc, unit="none", w=12, h=8, x=0, y=0,
               overrides=None, minv=None, maxv=None, fill=0, stack=False):
    """A per-device time series. Legend is Table/Right with Last and Max.

    Placement matters at this fleet's scale: a bottom legend with thirty
    entries pushes the plot into a sliver, while a right-hand table stays
    scannable and sortable.
    """
    custom = {
        "axisPlacement": "auto",
        "drawStyle": "line",
        "fillOpacity": fill,
        "lineWidth": 1,
        "pointSize": 4,
        "showPoints": "never",
        "spanNulls": True,
        "stacking": {"group": "A", "mode": "normal" if stack else "none"},
    }
    defaults = {"custom": custom, "unit": unit,
                "color": {"mode": "palette-classic"},
                "thresholds": {"mode": "absolute",
                               "steps": [{"color": "green", "value": None}]}}
    if minv is not None:
        defaults["min"] = minv
    if maxv is not None:
        defaults["max"] = maxv
    return {
        "id": nid(),
        "type": "timeseries",
        "title": title,
        "description": desc,
        "datasource": DS,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "targets": targets,
        "fieldConfig": {"defaults": defaults, "overrides": overrides or []},
        "options": {
            "legend": {"calcs": ["lastNotNull", "max"], "displayMode": "table",
                       "placement": "right", "showLegend": True},
            "tooltip": {"mode": "multi", "sort": "desc"},
        },
    }


def state_timeline(title, targets, desc, mappings, w=24, h=8, x=0, y=0):
    return {
        "id": nid(),
        "type": "state-timeline",
        "title": title,
        "description": desc,
        "datasource": DS,
        "gridPos": {"h": h, "w": w, "x": x, "y": y},
        "targets": targets,
        "fieldConfig": {
            "defaults": {
                "custom": {"fillOpacity": 80, "lineWidth": 0},
                "mappings": mappings,
                "thresholds": {"mode": "absolute",
                               "steps": [{"color": "text", "value": None}]},
            },
            "overrides": [],
        },
        "options": {
            "alignValue": "left",
            "legend": {"displayMode": "list", "placement": "bottom",
                       "showLegend": False},
            "mergeValues": True,
            "rowHeight": 0.9,
            "showValue": "never",
            "tooltip": {"mode": "single", "sort": "none"},
        },
    }


def row(title, panels, collapsed=True, y=0):
    return {
        "id": nid(),
        "type": "row",
        "title": title,
        "collapsed": collapsed,
        "gridPos": {"h": 1, "w": 24, "x": 0, "y": y},
        "panels": panels if collapsed else [],
    }


# Value mappings. Polarity is not shared between these two: for a warning
# flag 1 is the bad state, for an availability or success gauge 1 is the
# good one. The previous dashboard used the first mapping on all three
# panels, which painted a healthy fleet solid red.
MAP_FLAG = [{"type": "value", "options": {
    "0": {"text": "clear", "color": "green", "index": 0},
    "1": {"text": "SET", "color": "red", "index": 1}}}]

MAP_OK = [{"type": "value", "options": {
    "0": {"text": "FAIL", "color": "red", "index": 1},
    "1": {"text": "ok", "color": "green", "index": 0}}}]

MAP_PRESENT = [{"type": "value", "options": {
    "0": {"text": "absent", "color": "text", "index": 1},
    "1": {"text": "present", "color": "green", "index": 0}}}]


def organize(exclude=(), rename=None, order=None):
    return {"id": "organize", "options": {
        "excludeByName": {k: True for k in exclude},
        "renameByName": rename or {},
        "indexByName": {k: i for i, k in enumerate(order or [])},
    }}


# --- Row 1: fleet health -------------------------------------------------
# Every panel here is a scalar. `or vector(0)` matters: a count over an empty
# set returns no series at all, which Grafana renders as "No data" rather
# than as the zero it actually means.
fleet = [
    stat("Devices", f"count(nvme_logpage_scrape_success{{{SEL}}})",
         "NVMe controllers currently being scraped.", w=3, x=0, y=1,
         color_mode="none"),
    stat("Hosts", f"count(count by (instance) (nvme_logpage_scrape_success{{{SEL}}}))",
         "Hosts reporting at least one NVMe controller.", w=3, x=3, y=1,
         color_mode="none"),
    stat("Not live",
         f"count(nvme_logpage_controller_live{{{SEL}}} == 0) or vector(0)",
         "Controllers the kernel does not report as live. Such a controller "
         "refuses admin commands, so every read failure attributed to it "
         "follows from this rather than from the exporter. Counted on the "
         "boolean rather than on the state label: the labelled series is "
         "replaced when the state changes, so for one staleness window a "
         "recovered controller would still be counted here.",
         w=3, x=6, y=1,
         thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 1}],
         color_mode="background", text_size=40),
    stat("Unhealthy",
         f"count(max by (instance, device) (nvme_logpage_critical_warning_flag{{{SEL}}}) > 0) or vector(0)",
         "Devices with at least one Critical Warning bit set in the health log. "
         "This is the drive's own self-assessment, not a threshold anyone chose.",
         w=3, x=9, y=1,
         thresholds=[{"color": "green", "value": None}, {"color": "red", "value": 1}],
         color_mode="background", text_size=40),
    stat("Failing scrape",
         f"sum(nvme_logpage_scrape_success{{{SEL}}} == bool 0)",
         "Devices the exporter could not read this scrape. Check "
         "nvme_logpage_errors_total{reason} in the Exporter row: 'open' means "
         "device-file permissions, 'capability' means CAP_SYS_ADMIN.",
         w=3, x=12, y=1,
         thresholds=[{"color": "green", "value": None}, {"color": "red", "value": 1}],
         color_mode="background", text_size=40),
    stat("Over own temp threshold",
         f"count(nvme_logpage_composite_temperature_celsius{{{SEL}}} >= on(job, instance, device) "
         f"nvme_logpage_composite_temperature_warning_threshold_celsius{{{SEL}}}) or vector(0)",
         "Devices at or above the warning temperature the controller itself "
         "reports. Compared against each drive's own threshold, not a fixed "
         "number: across this fleet those thresholds span 17 degrees, from "
         "69.9 C on Intel and Dell to 86.9 C on some Samsung models.",
         w=3, x=15, y=1,
         thresholds=[{"color": "green", "value": None}, {"color": "orange", "value": 1}],
         color_mode="background", text_size=40),
    stat("Worst endurance used",
         f"max(nvme_logpage_endurance_used_ratio{{{SEL}}})",
         "Highest consumed write endurance in the selection. The per-drive "
         "breakdown is in the table below.",
         unit="percentunit", w=3, x=18, y=1,
         thresholds=[{"color": "green", "value": None},
                     {"color": "yellow", "value": 0.7},
                     {"color": "orange", "value": 0.85},
                     {"color": "red", "value": 1.0}],
         color_mode="background", text_size=40),
    stat("Reporting OCP",
         f'count(nvme_logpage_supported{{{SEL},page="0xc0"}} == 1) or vector(0)',
         "Devices exposing the OCP extended health log (page 0xC0). Only "
         "these can report true write amplification and physical media "
         "writes; the split is by drive class, not by vendor.",
         w=3, x=21, y=1, color_mode="none"),
]

pie_models = {
    "id": nid(),
    "type": "piechart",
    "title": "Devices by model",
    "description": "Model mix in the current selection.",
    "datasource": DS,
    "gridPos": {"h": 8, "w": 8, "x": 0, "y": 5},
    "targets": [tgt(f"count by (model) (nvme_logpage_device_info{{{SEL}}})",
                    legend="{{model}}", instant=True)],
    "fieldConfig": {"defaults": {"custom": {"hideFrom": {}},
                                 "mappings": []}, "overrides": []},
    "options": {
        "displayLabels": ["percent"],
        "legend": {"displayMode": "table", "placement": "right",
                   "showLegend": True, "values": ["value"]},
        "pieType": "donut",
        "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
    },
}

# Devices with a warning bit set. In a healthy fleet this is empty, and an
# empty panel is the point: it reads as "nothing wrong" at a glance, the way
# the smartctl dashboard's Critical Warnings tile does.
warnings_table = table(
    "Critical warnings — devices needing attention",
    [tgt(f"nvme_logpage_critical_warning_flag{{{SEL}}} == 1", instant=True, fmt="table")],
    "Only devices with a bit set appear here. Empty is the healthy state. "
    "Flags: spare_below_threshold, temperature, reliability_degraded, "
    "read_only, volatile_backup_failed, persistent_memory_ro.",
    transformations=[
        organize(exclude=["Time", "__name__", "job", "Value"],
                 rename={"instance": "Host", "device": "Device",
                         "serial": "Serial", "flag": "Flag"},
                 order=["instance", "device", "serial", "flag"]),
    ],
    w=16, h=8, x=8, y=5,
)


# --- Row 2: worst offenders ---------------------------------------------
# Bounded by $top_n, so these tables are the same size on one host and on
# five hundred. `group_left(model)` carries the model across from the info
# metric, which is the only place it exists.
def with_model(expr):
    return (f"{expr} * on(job, instance, device) group_left(model) "
            f"nvme_logpage_device_info{{{SEL}}}")


worst_wear = table(
    "Most worn drives",
    [tgt(f"topk($top_n, {with_model(f'nvme_logpage_endurance_used_ratio{{{SEL}}}')})",
         instant=True, fmt="table")],
    "Consumed write endurance, worst first. The drive's own estimate: it may "
    "exceed 100%, which the specification allows.",
    transformations=[
        organize(exclude=["Time", "__name__", "job"],
                 rename={"instance": "Host", "device": "Device",
                         "serial": "Serial", "model": "Model",
                         "Value": "Endurance used"},
                 order=["instance", "device", "model", "serial", "Value"]),
    ],
    overrides=[{
        "matcher": {"id": "byName", "options": "Endurance used"},
        "properties": [
            {"id": "unit", "value": "percentunit"},
            {"id": "custom.cellOptions",
             "value": {"type": "gauge", "mode": "gradient"}},
            {"id": "max", "value": 1},
            {"id": "min", "value": 0},
            {"id": "thresholds", "value": {"mode": "absolute", "steps": [
                {"color": "green", "value": None},
                {"color": "yellow", "value": 0.7},
                {"color": "orange", "value": 0.85},
                {"color": "red", "value": 1.0}]}},
        ],
    }],
    w=12, h=9, x=0, y=14, sort_by="Endurance used",
)

least_headroom = table(
    "Least temperature headroom",
    [tgt(f"bottomk($top_n, {with_model(f'(nvme_logpage_composite_temperature_warning_threshold_celsius{{{SEL}}} - on(job, instance, device) nvme_logpage_composite_temperature_celsius{{{SEL}}})')})",
         instant=True, fmt="table")],
    "Degrees left before each drive reaches the warning temperature it "
    "reports for itself. A negative value means the drive is already over "
    "its own threshold. Ranking by headroom rather than by temperature is "
    "what makes a mixed fleet comparable: the thresholds differ by 17 "
    "degrees across these models.",
    transformations=[
        organize(exclude=["Time", "__name__", "job"],
                 rename={"instance": "Host", "device": "Device",
                         "serial": "Serial", "model": "Model",
                         "Value": "Headroom"},
                 order=["instance", "device", "model", "serial", "Value"]),
    ],
    overrides=[{
        "matcher": {"id": "byName", "options": "Headroom"},
        "properties": [
            {"id": "unit", "value": "celsius"},
            {"id": "custom.cellOptions",
             "value": {"type": "color-text"}},
            {"id": "thresholds", "value": {"mode": "absolute", "steps": [
                {"color": "red", "value": None},
                {"color": "orange", "value": 5},
                {"color": "yellow", "value": 15},
                {"color": "green", "value": 25}]}},
        ],
    }],
    w=12, h=9, x=12, y=14, sort_by="Headroom", sort_desc=False,
)

worst_wa = table(
    "Highest write amplification",
    [tgt(f"topk($top_n, {with_model(f'(nvme_logpage_media_written_bytes_total{{{SEL}}} / on(job, instance, device) nvme_logpage_written_bytes_total{{{SEL}}})')})",
         instant=True, fmt="table")],
    "Bytes written to the NAND divided by bytes sent by the host — lifetime "
    "average, from the OCP log. Only drives exposing page 0xC0 appear. A "
    "high ratio on a drive that has written very little is normal: at low "
    "cumulative writes the figure is dominated by the controller's own "
    "background activity rather than by the workload.",
    transformations=[
        organize(exclude=["Time", "__name__", "job"],
                 rename={"instance": "Host", "device": "Device",
                         "serial": "Serial", "model": "Model",
                         "Value": "Write amplification"},
                 order=["instance", "device", "model", "serial", "Value"]),
    ],
    overrides=[{
        "matcher": {"id": "byName", "options": "Write amplification"},
        "properties": [
            {"id": "unit", "value": "none"},
            {"id": "decimals", "value": 2},
            {"id": "custom.cellOptions", "value": {"type": "color-text"}},
            {"id": "thresholds", "value": {"mode": "absolute", "steps": [
                {"color": "green", "value": None},
                {"color": "yellow", "value": 2},
                {"color": "orange", "value": 4}]}},
        ],
    }],
    w=12, h=9, x=0, y=23, sort_by="Write amplification",
)

fastest_wear = table(
    "Fastest wearing drives",
    [tgt(f"topk($top_n, {with_model(f'(nvme_logpage_endurance_used_ratio{{{SEL}}} * 86400 * 365 / nvme_logpage_power_on_seconds_total{{{SEL}}})')})",
         instant=True, fmt="table")],
    "Endurance consumed per year, averaged over the drive's whole power-on "
    "life. Answers 'which drive will need replacing first', which the "
    "absolute wear figure does not: a drive at 50% after eight years matters "
    "less than one at 10% after six months. A lifetime average rather than a "
    "recent rate, because the field is whole percents and does not move at "
    "all over a short window.",
    transformations=[
        organize(exclude=["Time", "__name__", "job"],
                 rename={"instance": "Host", "device": "Device",
                         "serial": "Serial", "model": "Model",
                         "Value": "Wear per year"},
                 order=["instance", "device", "model", "serial", "Value"]),
    ],
    overrides=[{
        "matcher": {"id": "byName", "options": "Wear per year"},
        "properties": [
            {"id": "unit", "value": "percentunit"},
            {"id": "custom.cellOptions", "value": {"type": "color-text"}},
            {"id": "thresholds", "value": {"mode": "absolute", "steps": [
                {"color": "green", "value": None},
                {"color": "yellow", "value": 0.15},
                {"color": "orange", "value": 0.25},
                {"color": "red", "value": 0.5}]}},
        ],
    }],
    w=12, h=9, x=12, y=23, sort_by="Wear per year",
)

# --- Inventory (collapsed) ----------------------------------------------
# The join that the previous dashboard got wrong. Six queries whose label
# sets differ cannot be combined with `merge`, which appends frames instead
# of joining them: the result was every device listed once per metric, each
# time with a different column populated. joinByLabels joins on the labels
# the queries genuinely share.
# Columns for the device table. Each query is decorated twice before it can
# be joined:
#
#   * group_left(model, firmware) pulls the identifying labels across from
#     nvme_logpage_device_info, which is the only series carrying them. Without
#     this the frames have different label sets and cannot be joined at all.
#   * label_replace stamps a `metric` label, which joinByLabels uses to name
#     the resulting column. The arithmetic above drops __name__, so there is
#     nothing else left to name columns by.
#
# joinByLabels then has to join on every remaining label. Any label left out
# splits one device across several rows — which is the failure the previous
# version shipped, by way of a `merge` that appended frames instead.
DEVICE_JOIN_LABELS = ["instance", "device", "serial", "model", "firmware", "job"]

DEVICE_COLUMNS = [
    ("capacity", f"nvme_logpage_capacity_bytes{{{SEL}}}", "Capacity", "bytes", None),
    ("age", f"nvme_logpage_power_on_seconds_total{{{SEL}}} / 86400", "Age (days)", "d", 0),
    ("endurance", f"nvme_logpage_endurance_used_ratio{{{SEL}}}", "Endurance used",
     "percentunit", None),
    ("spare", f"nvme_logpage_available_spare_ratio{{{SEL}}}", "Spare", "percentunit", None),
    ("temp", f"nvme_logpage_composite_temperature_celsius{{{SEL}}}", "Temp", "celsius", None),
    ("media_errors", f"nvme_logpage_media_errors_total{{{SEL}}}", "Media errors", "none", 0),
]


def device_column_target(key, expr, ref):
    decorated = (f"({expr}) * on(job, instance, device) group_left(model, firmware) "
                 f"nvme_logpage_device_info{{{SEL}}}")
    return tgt(f'label_replace({decorated}, "metric", "{key}", "", "")',
               instant=True, fmt="table", ref=ref)


inventory_table = table(
    "Devices",
    [device_column_target(k, e, chr(65 + i))
     for i, (k, e, _, _, _) in enumerate(DEVICE_COLUMNS)],
    "One row per controller. See the generator for why each query is wrapped "
    "in group_left and label_replace: the columns cannot be joined without "
    "both.",
    transformations=[
        {"id": "joinByLabels", "options": {"value": "metric",
                                           "join": DEVICE_JOIN_LABELS}},
        organize(
            exclude=["job"],
            rename={"instance": "Host", "device": "Device", "serial": "Serial",
                    "model": "Model", "firmware": "Firmware",
                    **{k: title for k, _, title, _, _ in DEVICE_COLUMNS}},
            order=["instance", "device", "model", "firmware", "serial"]
                  + [k for k, _, _, _, _ in DEVICE_COLUMNS],
        ),
    ],
    overrides=[
        {"matcher": {"id": "byName", "options": title},
         "properties": ([{"id": "unit", "value": unit}]
                        + ([{"id": "decimals", "value": dec}] if dec is not None else []))}
        for _, _, title, unit, dec in DEVICE_COLUMNS
    ],
    w=24, h=12, x=0, y=0, sort_by="Endurance used",
)

firmware_table = table(
    "Firmware slots",
    [tgt(f"nvme_logpage_firmware_slot_info{{{SEL}}}", instant=True, fmt="table")],
    "Firmware held in each slot. A revision in slot 1 that differs from the "
    "active slot is the drive's previous firmware, kept for rollback — "
    "useful when auditing which drives have been updated and which have not.",
    transformations=[
        organize(exclude=["Time", "__name__", "job", "Value"],
                 rename={"instance": "Host", "device": "Device",
                         "serial": "Serial", "slot": "Slot",
                         "revision": "Revision"},
                 order=["instance", "device", "serial", "slot", "revision"]),
    ],
    w=12, h=9, x=0, y=12,
)

namespaces_table = table(
    "Namespaces",
    [
        tgt(f'label_replace(nvme_logpage_namespace_size_bytes{{{SEL}}}, "metric", "size", "", "")',
            instant=True, fmt="table", ref="A"),
        tgt(f'label_replace(nvme_logpage_namespace_used_bytes{{{SEL}}}, "metric", "used", "", "")',
            instant=True, fmt="table", ref="B"),
        tgt(f'label_replace(nvme_logpage_namespace_used_bytes{{{SEL}}} / '
            f'nvme_logpage_namespace_size_bytes{{{SEL}}}, "metric", "ratio", "", "")',
            instant=True, fmt="table", ref="C"),
    ],
    "Namespace size against the OCP NUSE field, which counts allocated LBAs. "
    "The gap between this and the filesystem's own usage is space the "
    "filesystem has freed but never handed back to the drive with TRIM — "
    "nothing else on the host reports that.",
    transformations=[
        {"id": "joinByLabels", "options": {
            "value": "metric",
            "join": ["instance", "device", "namespace", "serial", "job"]}},
        organize(exclude=["job"],
                 rename={"instance": "Host", "device": "Device",
                         "namespace": "Namespace", "serial": "Serial",
                         "size": "Size", "used": "Used (NUSE)",
                         "ratio": "Used ratio"},
                 order=["instance", "device", "namespace", "serial",
                        "size", "used", "ratio"]),
    ],
    overrides=[
        {"matcher": {"id": "byRegexp", "options": "Size|Used \\(NUSE\\)"},
         "properties": [{"id": "unit", "value": "bytes"}]},
        {"matcher": {"id": "byName", "options": "Used ratio"},
         "properties": [{"id": "unit", "value": "percentunit"}]},
    ],
    w=12, h=9, x=12, y=12,
)

# --- Collapsed detail rows ----------------------------------------------
endurance = [
    timeseries("Endurance used",
               [tgt(f"nvme_logpage_endurance_used_ratio{{{SEL}}}", "{{instance}} {{device}}")],
               "Consumed write endurance over time. The slope is what matters; "
               "the absolute value is the drive's own estimate.",
               unit="percentunit", minv=0, w=12, x=0, y=0),
    timeseries("Available spare vs threshold",
               [tgt(f"nvme_logpage_available_spare_ratio{{{SEL}}}", "{{instance}} {{device}} spare"),
                tgt(f"nvme_logpage_available_spare_threshold_ratio{{{SEL}}}",
                    "{{instance}} {{device}} threshold", ref="B")],
               "Remaining spare capacity against the threshold below which the "
               "controller raises its own warning.",
               unit="percentunit", minv=0, w=12, x=12, y=0),
    timeseries("Host write rate",
               [tgt(f"rate(nvme_logpage_written_bytes_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}}")],
               "Bytes per second written by the host.",
               unit="Bps", w=12, x=0, y=8),
    timeseries("Media write rate (OCP)",
               [tgt(f"rate(nvme_logpage_media_written_bytes_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}}")],
               "Bytes per second actually written to the NAND. Only drives "
               "exposing page 0xC0 appear here; compare against the host write "
               "rate beside it to see amplification as it happens rather than "
               "as a lifetime average.",
               unit="Bps", w=12, x=12, y=8),
    timeseries("Write amplification (lifetime)",
               [tgt(f"nvme_logpage_media_written_bytes_total{{{SEL}}} / on(job, instance, device) "
                    f"nvme_logpage_written_bytes_total{{{SEL}}}", "{{instance}} {{device}}")],
               "Media bytes divided by host bytes since the drive was new.",
               unit="none", w=12, x=0, y=16),
    timeseries("Projected time to endurance exhaustion",
               [tgt(f"nvme_logpage_power_on_seconds_total{{{SEL}}} * "
                    f"(1 - nvme_logpage_endurance_used_ratio{{{SEL}}}) / "
                    f"(nvme_logpage_endurance_used_ratio{{{SEL}}} > 0 < 1)",
                    "{{instance}} {{device}}")],
               "Time until 100% if the drive keeps wearing at its lifetime "
               "average. Drives that report no wear yet, and drives already "
               "past 100%, have nothing to project and are left out.",
               unit="s", w=12, x=12, y=16),
]

temperature = [
    timeseries("Composite temperature vs the drive's own thresholds",
               [tgt(f"nvme_logpage_composite_temperature_celsius{{{SEL}}}", "{{instance}} {{device}}"),
                tgt(f"nvme_logpage_composite_temperature_warning_threshold_celsius{{{SEL}}}",
                    "{{instance}} {{device}} warn", ref="B"),
                tgt(f"nvme_logpage_composite_temperature_critical_threshold_celsius{{{SEL}}}",
                    "{{instance}} {{device}} crit", ref="C")],
               "Thresholds come from Identify Controller, per drive. They span "
               "17 degrees across this fleet, so a fixed line drawn at 70 C "
               "would be at the normal operating point for some models and far "
               "above it for others.",
               unit="celsius", w=24, x=0, y=0),
    timeseries("Per-sensor temperature",
               [tgt(f"nvme_logpage_temperature_celsius{{{SEL}}}",
                    "{{instance}} {{device}} sensor {{sensor}}")],
               "Individual sensors, where the controller implements them. "
               "Sensors reading zero are not implemented and are not emitted.",
               unit="celsius", w=12, x=0, y=8),
    timeseries("Time fraction above temperature thresholds",
               [tgt(f"rate(nvme_logpage_warning_temperature_seconds_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} warning"),
                tgt(f"rate(nvme_logpage_critical_temperature_seconds_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} critical", ref="B")],
               "Fraction of wall-clock time spent over each threshold. A "
               "sustained value near 1 means the drive is permanently hot.",
               unit="percentunit", w=12, x=12, y=8),
    timeseries("Thermal throttling",
               [tgt(f"nvme_logpage_thermal_throttle_ratio{{{SEL}}}", "{{instance}} {{device}} throttle (OCP)"),
                tgt(f"rate(nvme_logpage_thermal_seconds_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} level {{level}}", ref="B")],
               "Throttling currently applied, from the OCP log, alongside the "
               "time fraction spent in each thermal management level from the "
               "health log. Both read zero on every drive checked so far.",
               unit="percentunit", w=24, x=0, y=16),
]

reliability = [
    state_timeline("Critical warning flags",
                   [tgt(f"nvme_logpage_critical_warning_flag{{{SEL}}} == 1",
                        "{{instance}} {{device}} {{flag}}")],
                   "Only flags that are actually set are plotted — an unfiltered "
                   "version draws one lane per device per flag and is unreadable "
                   "past a handful of hosts. Empty means nothing is set.",
                   MAP_FLAG, w=24, x=0, y=0),
    timeseries("Media errors (rate)",
               [tgt(f"rate(nvme_logpage_media_errors_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}}")],
               "Unrecovered data integrity errors per second. Any sustained "
               "non-zero value is serious.",
               unit="none", w=8, x=0, y=8),
    timeseries("Uncorrectable read errors (rate, OCP)",
               [tgt(f"rate(nvme_logpage_uncorrectable_read_errors_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}}")],
               "From the OCP log: reads the controller could not correct.",
               unit="none", w=8, x=8, y=8),
    timeseries("Retired NAND blocks (OCP)",
               [tgt(f"nvme_logpage_bad_nand_blocks_total{{{SEL}}}",
                    "{{instance}} {{device}} {{area}}")],
               "Blocks the controller has taken out of service. A field a "
               "controller reports as unimplemented is omitted entirely rather "
               "than shown as zero, so a missing series here means 'not "
               "reported', not 'none'.",
               unit="none", w=8, x=16, y=8),
    timeseries("Unsafe shutdowns and power cycles (rate)",
               [tgt(f"rate(nvme_logpage_unsafe_shutdowns_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} unsafe"),
                tgt(f"rate(nvme_logpage_power_cycles_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} cycles", ref="B")],
               "An unsafe shutdown count that tracks the power cycle count "
               "means the host is not shutting the drive down cleanly.",
               unit="none", w=12, x=0, y=16),
    table("Error log entries by status",
          [tgt(f"nvme_logpage_error_log_retained_entries{{{SEL}}}", instant=True, fmt="table")],
          "The most recent entries in the drive's Error Information log, "
          "grouped by status — at most 8, since the exporter reads a fixed "
          "512 bytes while the log itself holds 64 to 256. Diagnostic only. "
          "Note that this includes admin commands rejected because the drive "
          "does not implement them — including probes from other monitoring "
          "tools on the same host — so it is not a health signal on its own.",
          transformations=[
              organize(exclude=["Time", "__name__", "job"],
                       rename={"instance": "Host", "device": "Device",
                               "serial": "Serial",
                               "status_code_type": "SCT", "status_code": "SC",
                               "Value": "Entries"}),
          ],
          w=12, h=8, x=12, y=16),
]

activity = [
    timeseries("Host byte rate",
               [tgt(f"rate(nvme_logpage_read_bytes_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} read"),
                tgt(f"rate(nvme_logpage_written_bytes_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} write", ref="B")],
               "Throughput as the controller counts it.", unit="Bps",
               w=12, x=0, y=0),
    timeseries("Host command rate",
               [tgt(f"rate(nvme_logpage_host_read_commands_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} read"),
                tgt(f"rate(nvme_logpage_host_write_commands_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} write", ref="B")],
               "Commands per second. Divided into the byte rate beside it this "
               "gives average request size.",
               unit="none", w=12, x=12, y=0),
    timeseries("Controller busy fraction",
               [tgt(f"rate(nvme_logpage_controller_busy_seconds_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}}")],
               "Fraction of time the controller spent processing commands. "
               "Approaching 1 means the drive, not the host, is the bottleneck.",
               unit="percentunit", w=12, x=0, y=8),
    timeseries("Unaligned I/O (rate, OCP)",
               [tgt(f"rate(nvme_logpage_unaligned_io_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}}")],
               "Host operations not aligned to the controller's internal "
               "granularity. A high rate is a direct cause of write "
               "amplification.",
               unit="none", w=12, x=12, y=8),
]

exporter = [
    timeseries("Devices failing to scrape",
               [tgt(f"count(nvme_logpage_scrape_success{{{SEL}}} == 0) or vector(0)",
                    "failing")],
               "A count, not one lane per device: at fleet scale a per-device "
               "state timeline is an unreadable smear. Flat zero is healthy; "
               "the table beside it names whoever is not.",
               unit="none", minv=0, w=12, x=0, y=0, fill=20),
    table("Devices failing right now",
          [tgt(f"nvme_logpage_scrape_success{{{SEL}}} == 0", instant=True, fmt="table")],
          "Empty is the healthy state. Cross-reference the reason in "
          "\"Errors by reason\" below: 'open' means device-file permissions, "
          "'capability' means CAP_SYS_ADMIN.",
          transformations=[
              organize(exclude=["Time", "__name__", "job", "Value"],
                       rename={"instance": "Host", "device": "Device",
                               "serial": "Serial"}),
          ],
          w=12, h=8, x=12, y=0),
    timeseries("Devices serving each log page",
               [tgt(f"count by (page) (nvme_logpage_supported{{{SEL}}} == 1)",
                    "page {{page}}")],
               "One line per page rather than one per device and page, which "
               "is 112 lanes on this fleet alone. Pages 0x01, 0x02 and 0x03 "
               "are mandatory, so those lines should track the device count; "
               "0xC0 is expected to be lower, since client-class drives and "
               "the Intel and Dell units do not serve it.",
               unit="none", minv=0, w=24, x=0, y=8),
    timeseries("Errors by reason (rate)",
               [tgt(f"rate(nvme_logpage_errors_total{{{SEL}}}[$__rate_interval])",
                    "{{instance}} {{device}} {{reason}}")],
               "\"No data\" here means no device has ever failed, which is the "
               "healthy state. 'open' means the device file could not be "
               "opened — install the udev rule. 'capability' means it opened "
               "but the ioctl was refused — check AmbientCapabilities.",
               unit="none", w=12, x=0, y=16),
    timeseries("Scrape duration",
               [tgt(f"nvme_logpage_scrape_duration_seconds{{{SEL}}}", "{{instance}} {{device}}")],
               "Time to poll one device. Single-digit milliseconds is normal; "
               "this exporter issues ioctls directly rather than forking a "
               "helper per scrape.",
               unit="s", w=12, x=12, y=16),
    table("Exporter build",
          [tgt(f"nvme_logpage_build_info{{job=~\"$job\",instance=~\"$instance\"}}",
               instant=True, fmt="table")],
          "Version of the exporter running on each host — check here first "
          "when a metric is missing on some hosts but not others. The build-tag "
          "field is dropped: this project uses none, so it always reads "
          "\"unknown\".",
          transformations=[
              organize(exclude=["Time", "__name__", "job", "Value", "tags", "branch"],
                       rename={"instance": "Host"}),
          ],
          w=24, h=8, x=0, y=24),
]

panels = []
panels.append(row("Fleet health", [], collapsed=False, y=0))
panels.extend(fleet)
panels.append(pie_models)
panels.append(warnings_table)
panels.append(row("Worst offenders", [], collapsed=False, y=13))
panels.extend([worst_wear, least_headroom, worst_wa, fastest_wear])
panels.append(row("Inventory", [inventory_table, firmware_table, namespaces_table], y=32))
panels.append(row("Endurance and wear", endurance, y=33))
panels.append(row("Temperature", temperature, y=34))
panels.append(row("Reliability", reliability, y=35))
panels.append(row("Activity", activity, y=36))
panels.append(row("Exporter self-diagnostics", exporter, y=37))

dashboard = {
    "annotations": {"list": []},
    "editable": True,
    "graphTooltip": 1,
    "links": [],
    "panels": panels,
    "refresh": "1m",
    "schemaVersion": 39,
    "tags": ["nvme", "storage"],
    "templating": {"list": [
        {"name": "datasource", "type": "datasource", "query": "prometheus",
         "label": "Datasource", "hide": 0, "refresh": 1, "current": {}},
        {"name": "job", "type": "query", "datasource": DS,
         "query": "label_values(nvme_logpage_scrape_success, job)",
         "label": "Job", "multi": True, "includeAll": True, "allValue": ".*",
         "refresh": 1, "current": {"text": "All", "value": "$__all"}},
        {"name": "instance", "type": "query", "datasource": DS,
         "query": 'label_values(nvme_logpage_scrape_success{job=~"$job"}, instance)',
         "label": "Host", "multi": True, "includeAll": True, "allValue": ".*",
         "refresh": 1, "current": {"text": "All", "value": "$__all"}},
        {"name": "device", "type": "query", "datasource": DS,
         "query": 'label_values(nvme_logpage_scrape_success{job=~"$job",instance=~"$instance"}, device)',
         "label": "Device", "multi": True, "includeAll": True, "allValue": ".*",
         "refresh": 1, "current": {"text": "All", "value": "$__all"}},
        {"name": "top_n", "type": "custom", "label": "Top N",
         "query": "5,10,20,50,100",
         "options": [{"text": n, "value": n, "selected": n == "10"}
                     for n in ["5", "10", "20", "50", "100"]],
         "current": {"text": "10", "value": "10"}},
    ]},
    "time": {"from": "now-24h", "to": "now"},
    "timepicker": {},
    "timezone": "browser",
    "title": "NVMe logpage exporter",
    "uid": "nvme-logpage-exporter",
    "version": 1,
    "description": (
        "NVMe health from log pages read directly over ioctl. The top two "
        "rows are aggregates and topk tables, so they stay the same size on "
        "a fleet of five hundred hosts as on one; per-device detail is in the "
        "collapsed rows below and is meant to be reached by selecting a host."
    ),
}

with open(OUT, "w") as f:
    json.dump(dashboard, f, indent=2)
    f.write("\n")

n = sum(1 for p in panels if p.get("type") != "row")
n += sum(len(p.get("panels", [])) for p in panels if p.get("type") == "row")
print(f"wrote {os.path.normpath(OUT)}: {n} panels, "
      f"{sum(1 for p in panels if p.get('type') == 'row')} rows")
