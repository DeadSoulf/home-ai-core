#!/bin/sh
set -eu

BASE_URL=${HOME_AI_BASE_URL:-http://127.0.0.1:8080}
SERVICE=${HOME_AI_SERVICE:-home-ai-core.service}

echo "== Home-AI-Core server smoke test =="

if command -v systemctl >/dev/null 2>&1; then
  systemctl is-enabled "$SERVICE" >/dev/null
  systemctl is-active "$SERVICE" >/dev/null
fi

curl --fail --silent "$BASE_URL/health" >/tmp/home-ai-health.json
curl --fail --silent "$BASE_URL/" >/tmp/home-ai-index.html
curl --fail --silent "$BASE_URL/api/v1/security/setup-status" >/tmp/home-ai-setup.json
grep -q "Home-AI-Core" /tmp/home-ai-index.html

python3 - <<'PY'
import json
with open("/tmp/home-ai-health.json", "r", encoding="utf-8") as f:
    health = json.load(f)
assert health["status"] == "ok"
with open("/tmp/home-ai-setup.json", "r", encoding="utf-8") as f:
    setup = json.load(f)
assert isinstance(setup["initialized"], bool)
PY

test -f /var/lib/home-ai-core/core.db
test -f /var/lib/home-ai-core/identity/node-id

echo "Smoke test passed."
echo "Web UI: $BASE_URL/"
