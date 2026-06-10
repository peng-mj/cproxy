#!/usr/bin/env python3
"""scproxy integration test report generator."""

import json
import os
from datetime import datetime
from pathlib import Path

RESULTS_DIR = Path(os.environ.get("RESULTS_DIR", "/results"))
SUITES = {
    "apt": "apt_results.json",
    "pip": "pip_results.json",
    "go": "go_results.json",
    "npm": "npm_results.json",
    "cargo": "cargo_results.json",
    "github": "github_results.json",
}


def collect_results():
    suites = []
    for name, filename in SUITES.items():
        filepath = RESULTS_DIR / filename
        if filepath.exists():
            with open(filepath) as f:
                tests = json.load(f)
                suites.append({"name": name.upper(), "tests": tests})
    return suites


def calc_stats(suites):
    total_pass = total_fail = total_duration = 0
    for suite in suites:
        for t in suite["tests"]:
            total_duration += t.get("duration", 0)
            if t["status"] == "PASS":
                total_pass += 1
            else:
                total_fail += 1
    return total_pass, total_fail, total_duration


def generate_html(suites, total_pass, total_fail, total_duration):
    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    total = total_pass + total_fail
    rate = f"{total_pass / total * 100:.1f}" if total > 0 else "0.0"

    rows = ""
    for suite in suites:
        s_pass = sum(1 for t in suite["tests"] if t["status"] == "PASS")
        s_fail = sum(1 for t in suite["tests"] if t["status"] == "FAIL")
        s_dur = sum(t.get("duration", 0) for t in suite["tests"])
        rows += f'<h3>{suite["name"]} ({s_pass}/{s_pass + s_fail} passed, {s_dur}s)</h3>\n'
        rows += '<table><tr><th>Test</th><th>Status</th><th>Duration</th></tr>\n'
        for t in suite["tests"]:
            css = "pass" if t["status"] == "PASS" else "fail"
            icon = "&#x2705;" if t["status"] == "PASS" else "&#x274C;"
            rows += (
                f'<tr><td>{t["test"]}</td>'
                f'<td class="{css}">{icon} {t["status"]}</td>'
                f'<td>{t["duration"]}s</td></tr>\n'
            )
        rows += "</table>\n"

    html = f"""<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>scproxy Integration Test Report</title>
<style>
body {{ font-family: -apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif; margin:40px; background:#f5f5f5; }}
.c {{ max-width:1000px; margin:0 auto; background:#fff; padding:30px; border-radius:8px; box-shadow:0 2px 10px rgba(0,0,0,.1); }}
h1 {{ border-bottom:3px solid #007bff; padding-bottom:10px; }}
.cards {{ display:grid; grid-template-columns:repeat(4,1fr); gap:16px; margin:24px 0; }}
.card {{ padding:20px; border-radius:8px; text-align:center; color:#fff; }}
.card.pass {{ background:#28a745; }} .card.fail {{ background:#dc3545; }}
.card.time {{ background:#17a2b8; }} .card.rate {{ background:#6f42c1; }}
.card h2 {{ margin:0; font-size:2em; }} .card p {{ margin:4px 0 0; opacity:.9; }}
table {{ width:100%; border-collapse:collapse; margin:12px 0 24px; }}
th,td {{ padding:10px 12px; text-align:left; border-bottom:1px solid #ddd; }}
th {{ background:#f8f9fa; }} tr:hover {{ background:#f8f9fa; }}
td.pass {{ color:#155724; background:#d4edda; font-weight:600; }}
td.fail {{ color:#721c24; background:#f8d7da; font-weight:600; }}
</style></head><body><div class="c">
<h1>scproxy Integration Test Report</h1>
<p>Generated: {now}</p>
<div class="cards">
  <div class="card pass"><h2>{total_pass}</h2><p>Passed</p></div>
  <div class="card fail"><h2>{total_fail}</h2><p>Failed</p></div>
  <div class="card time"><h2>{total_duration}s</h2><p>Duration</p></div>
  <div class="card rate"><h2>{rate}%</h2><p>Pass Rate</p></div>
</div>
{rows}
</div></body></html>"""

    (RESULTS_DIR / "report.html").write_text(html, encoding="utf-8")


def generate_markdown(suites, total_pass, total_fail, total_duration):
    total = total_pass + total_fail
    rate = f"{total_pass / total * 100:.1f}" if total > 0 else "0.0"
    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    md = f"# scproxy Integration Test Report\n\n**Generated:** {now}\n\n"
    md += f"| Metric | Value |\n|---|---|\n| Total | {total} |\n| Passed | {total_pass} |\n| Failed | {total_fail} |\n| Duration | {total_duration}s |\n| Pass Rate | {rate}% |\n\n"

    for suite in suites:
        s_pass = sum(1 for t in suite["tests"] if t["status"] == "PASS")
        s_total = len(suite["tests"])
        md += f"## {suite['name']} ({s_pass}/{s_total})\n\n"
        md += "| Test | Status | Duration |\n|---|---|---|\n"
        for t in suite["tests"]:
            icon = ":white_check_mark:" if t["status"] == "PASS" else ":x:"
            md += f"| {t['test']} | {icon} {t['status']} | {t['duration']}s |\n"
        md += "\n"

    (RESULTS_DIR / "report.md").write_text(md, encoding="utf-8")


def generate_json_report(suites, total_pass, total_fail, total_duration):
    report = {
        "timestamp": datetime.now().isoformat(),
        "summary": {
            "total": total_pass + total_fail,
            "passed": total_pass,
            "failed": total_fail,
            "duration": total_duration,
        },
        "suites": suites,
    }
    (RESULTS_DIR / "report.json").write_text(
        json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8"
    )


if __name__ == "__main__":
    print("Collecting results...")
    suites = collect_results()
    tp, tf, td = calc_stats(suites)

    generate_html(suites, tp, tf, td)
    generate_markdown(suites, tp, tf, td)
    generate_json_report(suites, tp, tf, td)

    print(f"\nDone: {tp} passed, {tf} failed, {td}s total")
    print(f"Report: /results/report.html")
