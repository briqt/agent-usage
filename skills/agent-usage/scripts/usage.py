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

# Official per-token overrides: input, output, cache read, cache write 5m, cache write 1h.
# LiteLLM remains the broad fallback; these values preserve fields it cannot express.
OFFICIAL_PRICING = {
    "claude-opus-4-8": [5e-6, 25e-6, 0.5e-6, 6.25e-6, 10e-6],
    "claude-opus-4-7": [5e-6, 25e-6, 0.5e-6, 6.25e-6, 10e-6],
    "claude-opus-4-6": [5e-6, 25e-6, 0.5e-6, 6.25e-6, 10e-6],
    "claude-sonnet-4-6": [3e-6, 15e-6, 0.3e-6, 3.75e-6, 6e-6],
    "claude-haiku-4-5": [1e-6, 5e-6, 0.1e-6, 1.25e-6, 2e-6],
}

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
        _pricing_cache = _parse_pricing({})
        return _pricing_cache


def _parse_pricing(raw):
    """Parse LiteLLM and apply official product-specific overrides."""
    result = dict(OFFICIAL_PRICING)
    for model, info in raw.items():
        if not isinstance(info, dict):
            continue
        inp = info.get("input_cost_per_token")
        out = info.get("output_cost_per_token")
        if inp is None or out is None:
            continue
        cr = info.get("cache_read_input_token_cost", 0) or 0
        cc = info.get("cache_creation_input_token_cost", 0) or 0
        result[model] = [float(inp), float(out), float(cr), float(cc), float(cc)]
    return result
    result.update(OFFICIAL_PRICING)

# PLACEHOLDER_MATCH_MARKER

def match_pricing(model, all_prices, provider=""):
    """Deterministically match exact/provider-qualified model aliases."""
    candidates = []
    provider = provider.strip().lower().strip("/")
    if provider:
        candidates.append(f"{provider}/{model}")
    candidates.append(model)
    candidates.extend(prefix + model for prefix in
                      ["anthropic/", "openai/", "deepseek/", "gemini/", "google/",
                       "mistral/", "cohere/", "azure_ai/"])
    for candidate in candidates:
        if candidate in all_prices:
            return all_prices[candidate]

    def norm(value):
        value = value.strip().lower().removeprefix("models/")
        for dotted in ("4.8", "4.7", "4.6", "4.5", "5.6", "5.5", "5.4", "5.3", "5.2"):
            value = value.replace(dotted, dotted.replace(".", "-"))
        return value

    for candidate in candidates:
        for key in sorted(all_prices):
            if norm(candidate) == norm(key):
                return all_prices[key]
    return None


def calc_cost(input_t, output_t, cache_create, cache_read, prices,
              cache_create_5m=0, cache_create_1h=0):
    """Calculate API-equivalent USD from non-overlapping token components."""
    if not cache_create_5m and not cache_create_1h:
        cache_create_5m = cache_create
    cache_1h_price = prices[4] if len(prices) > 4 else prices[3]
    return (input_t * prices[0] + output_t * prices[1] +
            cache_read * prices[2] + cache_create_5m * prices[3] +
            cache_create_1h * cache_1h_price)

# PLACEHOLDER_PARSERS_MARKER

# ── JSONL Parsers ──


def iter_lines(path):
    """Yield text lines while guaranteeing the session file is closed."""
    with open(path, "r", errors="replace") as stream:
        yield from stream

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
                for line in iter_lines(f):
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
                    breakdown = usage.get("cache_creation", {}) or {}
                    record = {
                        "source": "claude", "model": model, "timestamp": ts,
                        "project": project, "session_id": entry.get("sessionId", f.stem),
                        "provider": "anthropic",
                        "input": usage.get("input_tokens", 0) or 0,
                        "output": usage.get("output_tokens", 0) or 0,
                        "cache_read": usage.get("cache_read_input_tokens", 0) or 0,
                        "cache_create": usage.get("cache_creation_input_tokens", 0) or 0,
                        "cache_create_5m": breakdown.get("ephemeral_5m_input_tokens", 0) or 0,
                        "cache_create_1h": breakdown.get("ephemeral_1h_input_tokens", 0) or 0,
                        "request_id": entry.get("requestId", ""),
                        "message_id": msg.get("id", ""),
                    }
                    records.append(record)
            except Exception:
                continue
    deduped, without_identity = {}, []
    for record in records:
        if record["request_id"] and record["message_id"]:
            key = (record["session_id"], record["request_id"], record["message_id"])
            score = sum(record.get(name, 0) for name in
                        ("input", "output", "cache_read", "cache_create"))
            previous = deduped.get(key)
            previous_score = sum(previous.get(name, 0) for name in
                                 ("input", "output", "cache_read", "cache_create")) if previous else -1
            if score > previous_score:
                deduped[key] = record
        else:
            without_identity.append(record)
    return without_identity + list(deduped.values())

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
                for line in iter_lines(f):
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
                        cached = tu.get("cached_input_tokens", 0) or 0
                        total_input = tu.get("input_tokens", 0) or 0
                        records.append({
                            "source": "codex", "model": model, "timestamp": ts,
                            "project": "", "session_id": session_id,
                            "provider": "openai",
                            "input": max(0, total_input - cached),
                            "output": tu.get("output_tokens", 0) or 0,
                            "cache_read": cached,
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
                for line in iter_lines(f):
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


def scan_pi(base_paths, from_dt, to_dt):
    """Parse Pi coding agent JSONL files (same format as OpenClaw)."""
    records = []
    for base in base_paths:
        base = Path(base).expanduser()
        if not base.exists():
            continue
        for f in base.rglob("*.jsonl"):
            try:
                project = f.parent.name
                session_id = f.stem
                for line in iter_lines(f):
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        entry = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    if entry.get("type") == "session":
                        session_id = entry.get("id", session_id)
                        cwd = entry.get("cwd", "")
                        if cwd:
                            project = Path(cwd).name
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
                        "source": "pi", "model": msg.get("model", ""),
                        "timestamp": ts, "project": project, "session_id": session_id,
                        "input": usage.get("input", 0) or 0,
                        "output": usage.get("output", 0) or 0,
                        "cache_read": usage.get("cacheRead", 0) or 0,
                        "cache_create": usage.get("cacheWrite", 0) or 0,
                    })
            except Exception:
                continue
    return records


def scan_opencode(db_paths, from_dt, to_dt):
    """Parse OpenCode SQLite database."""
    import sqlite3
    records = []
    for db_path in db_paths:
        db_path = Path(db_path).expanduser()
        if not db_path.exists():
            continue
        try:
            conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
            rows = conn.execute(
                "SELECT m.data, m.session_id, s.directory FROM message m "
                "JOIN session s ON m.session_id = s.id"
            ).fetchall()
            conn.close()
            for data_json, session_id, directory in rows:
                try:
                    msg = json.loads(data_json)
                except json.JSONDecodeError:
                    continue
                if msg.get("role") != "assistant" or not msg.get("modelID"):
                    continue
                tokens = msg.get("tokens", {})
                if tokens.get("input", 0) == 0 and tokens.get("output", 0) == 0:
                    continue
                time_info = msg.get("time", {})
                created_ms = time_info.get("created", 0)
                if not created_ms:
                    continue
                ts = datetime.fromtimestamp(created_ms / 1000)
                if not (from_dt <= ts.date() <= to_dt):
                    continue
                cache = tokens.get("cache", {})
                records.append({
                    "source": "opencode", "model": msg.get("modelID", ""),
                    "timestamp": ts, "project": directory or "", "session_id": session_id,
                    "provider": msg.get("providerID", ""),
                    "native_cost": msg.get("cost", 0) or 0,
                    "input": tokens.get("input", 0) or 0,
                    "output": tokens.get("output", 0) or 0,
                    "cache_read": cache.get("read", 0) or 0,
                    "cache_create": cache.get("write", 0) or 0,
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
    "opencode": ["~/.local/share/opencode/opencode.db"],
    "pi": ["~/.pi/agent/sessions"],
}

def collect(from_dt, to_dt, source=None):
    records = []
    if source in (None, "", "claude"):
        records += scan_claude(DEFAULT_PATHS["claude"], from_dt, to_dt)
    if source in (None, "", "codex"):
        records += scan_codex(DEFAULT_PATHS["codex"], from_dt, to_dt)
    if source in (None, "", "openclaw"):
        records += scan_openclaw(DEFAULT_PATHS["openclaw"], from_dt, to_dt)
    if source in (None, "", "opencode"):
        records += scan_opencode(DEFAULT_PATHS["opencode"], from_dt, to_dt)
    if source in (None, "", "pi"):
        records += scan_pi(DEFAULT_PATHS["pi"], from_dt, to_dt)
    return records


def enrich_costs(records):
    """Add deterministic API-equivalent estimates and pricing status."""
    prices = fetch_pricing()
    for r in records:
        p = match_pricing(r["model"], prices, r.get("provider", ""))
        r["priced"] = p is not None
        r["cost"] = calc_cost(
            r["input"], r["output"], r["cache_create"], r["cache_read"], p,
            r.get("cache_create_5m", 0), r.get("cache_create_1h", 0),
        ) if p else 0
    return records


# ── Commands ──

def cmd_stats(records):
    sessions = set(r["session_id"] for r in records)
    total_input = sum(r["input"] + r["cache_read"] + r["cache_create"] for r in records)
    cache_read = sum(r["cache_read"] for r in records)
    api_estimate = round(sum(r["cost"] for r in records), 4)
    return {
        "total_cost": api_estimate,
        "api_estimated_cost_usd": api_estimate,
        "actual_cost_usd": round(sum(r.get("native_cost", 0) for r in records), 4),
        "total_tokens": total_input + sum(r["output"] for r in records),
        "total_output_tokens": sum(r["output"] for r in records),
        "cache_hit_rate": round(cache_read / total_input, 4) if total_input else 0,
        "total_sessions": len(sessions),
        "total_calls": len(records),
        "unpriced_records": sum(not r.get("priced", False) for r in records),
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
        s["tokens"] += r["input"] + r["cache_read"] + r["cache_create"] + r["output"]
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
        m["tokens"] += r["input"] + r["cache_read"] + r["cache_create"] + r["output"]
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

