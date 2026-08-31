#!/usr/bin/env bash
# ==============================================================
# harbor-reset-robot-secret.sh
# Resets the k8s-harbor-sa robot account secret to the value
# specified in HARBOR_ROBOT_PASSWORD, using the Harbor REST API.
#
# Run this after harbor-setup.sh when harbor-validate.sh shows
# the robot failing the registry token service check.
#
# Usage:
#   HARBOR_ADMIN_PASSWORD=$(pulumi config get harbor:adminPassword) \
#   HARBOR_ROBOT_PASSWORD=$(pulumi config get harbor:robotPassword) \
#   bash scripts/harbor-reset-robot-secret.sh
# ==============================================================
set -euo pipefail

HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-5000}"
HARBOR_API="http://localhost:${HARBOR_LOCAL_PORT}/api/v2.0"
ROBOT_NAME="k8s-harbor-sa"
ROBOT_USER="robot\$${ROBOT_NAME}"

if [[ -z "${HARBOR_ADMIN_PASSWORD:-}" ]]; then
  echo "ERROR: HARBOR_ADMIN_PASSWORD env var is required." >&2
  exit 1
fi

if [[ -z "${HARBOR_ROBOT_PASSWORD:-}" ]]; then
  echo "ERROR: HARBOR_ROBOT_PASSWORD env var is required." >&2
  exit 1
fi

# ── Port-forward ──────────────────────────────────────────────────────────────
echo "==> Starting port-forward: localhost:${HARBOR_LOCAL_PORT} → harbor svc/harbor:80"
kubectl port-forward -n harbor svc/harbor "${HARBOR_LOCAL_PORT}:80" \
  > /tmp/harbor-portforward-reset.log 2>&1 &
PF_PID=$!
trap 'kill "${PF_PID}" 2>/dev/null || true' EXIT

for i in $(seq 1 10); do
  if nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then break; fi
  sleep 1
done
if ! nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then
  echo "ERROR: port-forward did not become ready on localhost:${HARBOR_LOCAL_PORT}." >&2
  exit 1
fi

# ── Find robot ID ─────────────────────────────────────────────────────────────
echo "==> Looking up robot '${ROBOT_NAME}'..."

robot_list=$(curl -s \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  "${HARBOR_API}/robots?page_size=100")

robot_id=$(echo "${robot_list}" \
  | grep -o "\"id\":[0-9]*[^}]*\"name\":\"robot\\\$${ROBOT_NAME}\"" \
  | grep -o '"id":[0-9]*' \
  | head -1 \
  | cut -d: -f2)

# Fallback: simpler grep if name appears before id in JSON
if [[ -z "${robot_id}" ]]; then
  robot_id=$(echo "${robot_list}" \
    | grep -o "\"name\":\"robot\\\$${ROBOT_NAME}\"[^}]*\"id\":[0-9]*" \
    | grep -o '"id":[0-9]*' \
    | head -1 \
    | cut -d: -f2)
fi

if [[ -z "${robot_id}" ]]; then
  echo "ERROR: Robot '${ROBOT_NAME}' not found. Run harbor-setup.sh first." >&2
  exit 1
fi

echo "    Found robot ID: ${robot_id}"

# ── PATCH /robots/{id} ───────────────────────────────────────────────────────
echo "==> Setting robot secret via PATCH /robots/${robot_id}..."

code=$(curl -s -o /tmp/harbor-reset-response.json -w "%{http_code}" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  -X PATCH "${HARBOR_API}/robots/${robot_id}" \
  -H "Content-Type: application/json" \
  -d "{\"secret\":\"${HARBOR_ROBOT_PASSWORD}\"}")

if [[ "${code}" != "200" ]]; then
  echo "ERROR: PATCH failed (HTTP ${code})." >&2
  cat /tmp/harbor-reset-response.json >&2
  echo "" >&2
  echo "Reset the robot secret manually in the Harbor UI." >&2
  exit 1
fi

echo "    PATCH succeeded."

# ── Verify ────────────────────────────────────────────────────────────────────
echo "==> Verifying robot credentials against registry token service..."

code=$(curl -s -o /dev/null -w "%{http_code}" \
  -u "${ROBOT_USER}:${HARBOR_ROBOT_PASSWORD}" \
  "http://localhost:${HARBOR_LOCAL_PORT}/service/token?service=harbor-registry")

if [[ "${code}" == "200" ]]; then
  echo "    Robot credentials verified — registry token service: OK"
  echo ""
  echo "==> Done. You can now run model-sync.sh."
else
  echo "ERROR: Credentials still rejected by registry token service (HTTP ${code})." >&2
  echo "       The password may not meet Harbor's complexity requirements." >&2
  echo "       Ensure HARBOR_ROBOT_PASSWORD contains uppercase, lowercase," >&2
  echo "       numbers, and at least one special character (!@#\$%^&*)." >&2
  exit 1
fi
