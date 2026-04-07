#!/usr/bin/env bash
# detect.sh — Check if agent-usage server is running
# Output: "API" if reachable, "LOCAL" otherwise
set -euo pipefail
if curl -sf --max-time 2 "http://localhost:9800/api/stats?from=2000-01-01&to=2099-01-01" > /dev/null 2>&1; then
  echo "API"
else
  echo "LOCAL"
fi
