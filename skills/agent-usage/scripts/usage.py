#!/usr/bin/env python3
"""agent-usage local JSONL parser — query AI coding agent usage without a server.

Usage:
    python3 usage.py stats [--from DATE] [--to DATE] [--source SOURCE]
    python3 usage.py cost-by-model [--from DATE] [--to DATE] [--source SOURCE]
    python3 usage.py sessions [--from DATE] [--to DATE] [--source SOURCE]
    python3 usage.py top-models [--from DATE] [--to DATE] [-n N]
"""
import argparse, json, os, sys
from collections import defaultdict
from datetime import datetime, date
from pathlib import Path

# ── Pricing: fetched from litellm at runtime ──

LITELLM_URL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
_pricing_cache = None

# PLACEHOLDER_APPEND_MARKER

def fetch_pricing():
    """Fetch pricing from litellm GitHub. Cache in memory and on disk (~/.cache/agent-usage/pricing.json)."""
    global _pricing_cache
    if _pricing_cache is not None:
        return _pricing_cache

    cache_dir = Path.home() / ".cache" / "agent-usage"
    cache_file = cache_dir / "pricing.json"

    # Try disk cache (< 24h old)
    if cache_file.exists():
        age = datetime.now().timestamp() - cache_file.stat().st_mtime
        if age < 86400:
            try:
                raw = json.loads(cache_file.read_text())
                _pricing_cache = _parse_pricing(raw)
                return _pricing_cache
            except Exception:
                pass

    # Fetch from network
    try:
        import urllib.request
        req = urllib.request.Request(LITELLM_URL, headers={"User-Agent": "agent-usage-skill/1.0"})
        with urllib.request.urlopen(req, timeout=15) as resp:
            raw = json.loads(resp.read())
        cache_dir.mkdir(parents=True, exist_ok=True)
        cache_file.write_text(json.dumps(raw))
        _pricing_cache = _parse_pricing(raw)
        return _pricing_cache
    except Exception as e:
        print(f"Warning: could not fetch pricing: {e}", file=sys.stderr)
        _pricing_cache = {}
        return _pricing_cache


def _parse_pricing(raw):
    """Parse litellm JSON into {model: [input, output, cache_read, cache_create]} per-token costs."""
    result = {}
    for model, info in raw.items():
        if not isinstance(info, dict):
            continue
        inp = info.get("input_cost_per_token")
        out = info.get("output_cost_per_token")
        if inp is None or out is None:
            continue
        cr = info.get("cache_read_input_token_cost", 0) or 0
        cc = info.get("cache_creation_input_token_cost", 0) or 0
        result[model] = [float(inp), float(out), float(cr), float(cc)]
    return result

# PLACEHOLDER_MATCH_MARKER

def match_pricing(model, all_prices):
    """Fuzzy match model name to pricing table. Mirrors Go matchPricing logic."""
    if model in all_prices:
        return all_prices[model]
    for prefix in ["anthropic/", "openai/", "deepseek/", "gemini/", "google/", "mistral/", "cohere/", "azure_ai/"]:
        if prefix + model in all_prices:
            return all_prices[prefix + model]

    def norm(s):
        return s.lower().replace("/", ".")

    mn = norm(model)
    mn_dash = mn.replace("4.6", "4-6").replace("4.5", "4-5").replace("3.5", "3-5").replace("5.4", "5-4")

    best_key, best_score = None, 0
    for k in all_prices:
        kn = norm(k)
        for m in (mn, mn_dash):
            if m in kn or kn in m:
                score = 10000 - len(k)
                if kn == m:
                    score += 100000
                if score > best_score:
                    best_key, best_score = k, score
    if best_key:
        return all_prices[best_key]
    return None


def calc_cost(input_t, output_t, cache_create, cache_read, prices):
    """Calculate USD cost. prices = [input, output, cache_read, cache_create]."""
    regular = max(0, input_t - cache_read - cache_create)
    return (regular * prices[0] + cache_create * prices[3] +
            cache_read * prices[2] + output_t * prices[1])

# PLACEHOLDER_PARSERS_MARKER

# ── JSONL Parsers ──

def parse_timestamp(ts):
    """Parse various timestamp formats to datetime."""
    if not ts:
        return None
    for fmt in ("%Y-%m-%dT%H:%M:%S.%fZ", "%Y-%m-%dT%H:%M:%SZ", "%Y-%m-%dT%H:%M:%S.%f%z", "%Y-%m-%dT%H:%M:%S%z"):
        try:
            return datetime.strptime(ts.rstrip("Z") + "Z" if "Z" not in ts and "+" not in ts else ts, fmt)
        except ValueError:
            continue
    try:
        return datetime.fromisoformat(ts.replace("Z", "+00:00"))
    except Exception:
        return None


def scan_claude(base_paths, from_dt, to_dt):
    """Parse Claude Code JSONL files."""
    records = []
    for base in base_paths:
        base = Path(base).expanduser()
        if not base.exists():
            continue
        for f in base.rglob("*.jsonl"):
            project = f.parent.name
            try:
                for line in open(f, "r", errors="replace"):
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        entry = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    if entry.get("type") != "assistant" or not entry.get("message"):
                        continue
                    msg = entry["message"] if isinstance(entry["message"], dict) else {}
                    usage = msg.get("usage", {})
                    if not usage or usage.get("cache_creation_input_tokens") is None:
                        continue
                    model = msg.get("model", "")
                    if model == "<synthetic>" or model == "delivery-mirror":
                        continue
                    ts = parse_timestamp(entry.get("timestamp", ""))
                    if not ts or not (from_dt <= ts.date() <= to_dt):
                        continue
                    records.append({
                        "source": "claude", "model": model, "timestamp": ts,
                        "project": project, "session_id": entry.get("sessionId", f.stem),
                        "input": usage.get("input_tokens", 0) or 0,
                        "output": usage.get("output_tokens", 0) or 0,
                        "cache_read": usage.get("cache_read_input_tokens", 0) or 0,
                        "cache_create": usage.get("cache_creation_input_tokens", 0) or 0,
                    })
            except Exception:
                continue
    return records

# PLACEHOLDER_CODEX_MARKER

def scan_codex(base_paths, from_dt, to_dt):
    """Parse Codex CLI JSONL files."""
    records = []
    for base in base_paths:
        base = Path(base).expanduser()
        if not base.exists():
            continue
        for f in base.rglob("*.jsonl"):
            try:
                session_id = f.stem
                model = ""
                for line in open(f, "r", errors="replace"):
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        entry = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    etype = entry.get("type", "")
                    payload = entry.get("payload") if isinstance(entry.get("payload"), dict) else {}
                    if etype == "session_meta":
                        session_id = payload.get("id", session_id)
                    elif etype == "turn_context":
                        model = payload.get("model", model)
                    elif etype == "event_msg" and payload.get("type") == "token_count":
                        info = payload.get("info", {}) or {}
                        tu = info.get("last_token_usage", {}) or {}
                        ts = parse_timestamp(entry.get("timestamp", ""))
                        if not ts or not (from_dt <= ts.date() <= to_dt):
                            continue
                        records.append({
                            "source": "codex", "model": model, "timestamp": ts,
                            "project": "", "session_id": session_id,
                            "input": tu.get("input_tokens", 0) or 0,
                            "output": tu.get("output_tokens", 0) or 0,
                            "cache_read": tu.get("cached_input_tokens", 0) or 0,
                            "cache_create": 0,
                        })
            except Exception:
                continue
    return records


def scan_openclaw(base_paths, from_dt, to_dt):
    """Parse OpenClaw JSONL files."""
    records = []
    for base in base_paths:
        base = Path(base).expanduser()
        if not base.exists():
            continue
        for f in base.rglob("*.jsonl"):
            try:
                agent_id = f.parent.parent.name
                session_id = f.stem
                for line in open(f, "r", errors="replace"):
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        entry = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    if entry.get("type") == "session":
                        session_id = entry.get("id", session_id)
                        continue
                    if entry.get("type") != "message":
                        continue
                    msg = entry.get("message", {}) or {}
                    if msg.get("role") != "assistant" or not msg.get("usage"):
                        continue
                    usage = msg["usage"]
                    ts = parse_timestamp(entry.get("timestamp", ""))
                    if not ts or not (from_dt <= ts.date() <= to_dt):
                        continue
                    records.append({
                        "source": "openclaw", "model": msg.get("model", ""),
                        "timestamp": ts, "project": agent_id, "session_id": session_id,
                        "input": usage.get("input", 0) or 0,
                        "output": usage.get("output", 0) or 0,
                        "cache_read": usage.get("cacheRead", 0) or 0,
                        "cache_create": usage.get("cacheWrite", 0) or 0,
                    })
            except Exception:
                continue
    return records

# PLACEHOLDER_COMMANDS_MARKER

# ── Collect all records ──

DEFAULT_PATHS = {
    "claude": ["~/.claude/projects"],
    "codex": ["~/.codex/sessions"],
    "openclaw": ["~/.openclaw/agents"],
}

def collect(from_dt, to_dt, source=None):
    records = []
    if source in (None, "", "claude"):
        records += scan_claude(DEFAULT_PATHS["claude"], from_dt, to_dt)
    if source in (None, "", "codex"):
        records += scan_codex(DEFAULT_PATHS["codex"], from_dt, to_dt)
    if source in (None, "", "openclaw"):
        records += scan_openclaw(DEFAULT_PATHS["openclaw"], from_dt, to_dt)
    return records


def enrich_costs(records):
    """Add cost_usd to each record using litellm pricing."""
    prices = fetch_pricing()
    for r in records:
        p = match_pricing(r["model"], prices)
        r["cost"] = calc_cost(r["input"], r["output"], r["cache_create"], r["cache_read"], p) if p else 0
    return records


# ── Commands ──

def cmd_stats(records):
    sessions = set(r["session_id"] for r in records)
    return {
        "total_cost": round(sum(r["cost"] for r in records), 4),
        "total_tokens": sum(r["input"] + r["output"] for r in records),
        "total_sessions": len(sessions),
        "total_calls": len(records),
    }


def cmd_cost_by_model(records):
    by_model = defaultdict(float)
    for r in records:
        by_model[r["model"]] += r["cost"]
    return sorted([{"model": m, "cost": round(c, 4)} for m, c in by_model.items()], key=lambda x: -x["cost"])


def cmd_sessions(records):
    sess = defaultdict(lambda: {"source": "", "project": "", "session_id": "", "tokens": 0, "cost": 0, "calls": 0, "start": None})
    for r in records:
        s = sess[r["session_id"]]
        s["source"] = r["source"]
        s["project"] = r["project"]
        s["session_id"] = r["session_id"]
        s["tokens"] += r["input"] + r["output"]
        s["cost"] += r["cost"]
        s["calls"] += 1
        if s["start"] is None or r["timestamp"] < s["start"]:
            s["start"] = r["timestamp"]
    result = []
    for s in sess.values():
        result.append({
            "session_id": s["session_id"], "source": s["source"], "project": s["project"],
            "tokens": s["tokens"], "total_cost": round(s["cost"], 4), "calls": s["calls"],
            "start_time": s["start"].isoformat() if s["start"] else "",
        })
    return sorted(result, key=lambda x: x["start_time"], reverse=True)


def cmd_top_models(records, n=5):
    by_model = defaultdict(lambda: {"tokens": 0, "cost": 0, "calls": 0})
    for r in records:
        m = by_model[r["model"]]
        m["tokens"] += r["input"] + r["output"]
        m["cost"] += r["cost"]
        m["calls"] += 1
    result = [{"model": k, **v} for k, v in by_model.items()]
    result.sort(key=lambda x: -x["cost"])
    return result[:n]


# ── Main ──

def main():
    parser = argparse.ArgumentParser(description="agent-usage local JSONL parser")
    parser.add_argument("command", choices=["stats", "cost-by-model", "sessions", "top-models"])
    parser.add_argument("--from", dest="from_date", default=date.today().isoformat())
    parser.add_argument("--to", dest="to_date", default=date.today().isoformat())
    parser.add_argument("--source", default="")
    parser.add_argument("-n", type=int, default=5)
    args = parser.parse_args()

    from_dt = date.fromisoformat(args.from_date)
    to_dt = date.fromisoformat(args.to_date)
    source = args.source or None

    records = enrich_costs(collect(from_dt, to_dt, source))

    if args.command == "stats":
        result = cmd_stats(records)
    elif args.command == "cost-by-model":
        result = cmd_cost_by_model(records)
    elif args.command == "sessions":
        result = cmd_sessions(records)
    elif args.command == "top-models":
        result = cmd_top_models(records, args.n)

    print(json.dumps(result, indent=2, default=str))


if __name__ == "__main__":
    main()

