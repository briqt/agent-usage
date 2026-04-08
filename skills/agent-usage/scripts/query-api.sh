#!/usr/bin/env bash
# query-api.sh — Wrapper for agent-usage REST API
# Usage: query-api.sh <command> [--from DATE] [--to DATE] [--source SRC] [--granularity G] [--session-id SID]
set -euo pipefail

BASE="http://localhost:9800/api"
CMD="${1:-stats}"; shift || true

FROM="" TO="" SOURCE="" GRAN="" SID=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --from) FROM="$2"; shift 2 ;;
    --to) TO="$2"; shift 2 ;;
    --source) SOURCE="$2"; shift 2 ;;
    --granularity) GRAN="$2"; shift 2 ;;
    --session-id) SID="$2"; shift 2 ;;
    *) shift ;;
  esac
done

# Default dates: today
TODAY=$(date +%Y-%m-%d)
FROM="${FROM:-$TODAY}"
TO="${TO:-$TODAY}"

build_qs() {
  local qs="from=${FROM}&to=${TO}"
  # Send local timezone offset (JS convention: UTC+8 = -480)
  local tz_offset
  tz_offset=$(date +%z | awk '{h=substr($0,1,3)+0; m=substr($0,4,2)+0; print -(h*60+m)}')
  qs="${qs}&tz_offset=${tz_offset}"
  [[ -n "$SOURCE" ]] && qs="${qs}&source=${SOURCE}"
  [[ -n "$GRAN" ]] && qs="${qs}&granularity=${GRAN}"
  echo "$qs"
}

case "$CMD" in
  stats|cost-by-model|cost-over-time|tokens-over-time|sessions)
    curl -sf "${BASE}/${CMD}?$(build_qs)" ;;
  session-detail)
    [[ -z "$SID" ]] && echo '{"error":"--session-id required"}' && exit 1
    curl -sf "${BASE}/session-detail?session_id=${SID}" ;;
  *) echo "{\"error\":\"unknown command: ${CMD}\"}" && exit 1 ;;
esac
