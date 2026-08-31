#!/usr/bin/env bash
# ==============================================================
# harbor-validate.sh
# Validates Harbor admin and robot account credentials against
# both the REST API and the registry token service (the auth
# path used by ORAS/Docker during image push/pull).
#
# Prerequisites:
#   - Harbor is running (pulumi up completed)
#   - kubectl configured for the target cluster
#
# Usage:
#   HARBOR_ADMIN_PASSWORD=$(pulumi config get harbor:adminPassword) \
#   HARBOR_ROBOT_PASSWORD=$(pulumi config get harbor:robotPassword) \
#   bash scripts/harbor-validate.sh
# ==============================================================
set -euo pipefail

HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-5000}"
HARBOR_API="http://localhost:${HARBOR_LOCAL_PORT}/api/v2.0"
ROBOT_USER="robot\$k8s-harbor-sa"

PASS="[ OK ]"
FAIL="[FAIL]"

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
  > /tmp/harbor-portforward-validate.log 2>&1 &
PF_PID=$!
trap 'kill "${PF_PID}" 2>/dev/null || true' EXIT

for i in $(seq 1 10); do
  if nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then break; fi
  sleep 1
done
if ! nc -z localhost "${HARBOR_LOCAL_PORT}" 2>/dev/null; then
  echo "ERROR: port-forward did not become ready on localhost:${HARBOR_LOCAL_PORT}." >&2
  cat /tmp/harbor-portforward-validate.log >&2
  exit 1
fi

echo ""
echo "==> Validating credentials against localhost:${HARBOR_LOCAL_PORT}"
echo ""

# ── Admin: REST API ───────────────────────────────────────────────────────────
code=$(curl -s -o /dev/null -w "%{http_code}" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  "${HARBOR_API}/projects")
if [[ "${code}" == "200" ]]; then
  echo "  ${PASS} Admin   — REST API (/api/v2.0/projects)"
else
  echo "  ${FAIL} Admin   — REST API (/api/v2.0/projects) → HTTP ${code}"
fi

# ── Admin: registry token service ────────────────────────────────────────────
code=$(curl -s -o /dev/null -w "%{http_code}" \
  -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  "http://localhost:${HARBOR_LOCAL_PORT}/service/token?service=harbor-registry")
if [[ "${code}" == "200" ]]; then
  echo "  ${PASS} Admin   — registry token service (/service/token)"
else
  echo "  ${FAIL} Admin   — registry token service (/service/token) → HTTP ${code}"
fi

echo ""

# ── Robot: REST API ───────────────────────────────────────────────────────────
code=$(curl -s -o /dev/null -w "%{http_code}" \
  -u "${ROBOT_USER}:${HARBOR_ROBOT_PASSWORD}" \
  "${HARBOR_API}/users/current")
if [[ "${code}" == "200" || "${code}" == "412" ]]; then
  echo "  ${PASS} Robot   — REST API (/api/v2.0/users/current) (HTTP ${code} — robot accounts return 412, not 401)"
else
  echo "  ${FAIL} Robot   — REST API (/api/v2.0/users/current) → HTTP ${code}"
fi

# ── Robot: registry token service ────────────────────────────────────────────
code=$(curl -s -o /dev/null -w "%{http_code}" \
  -u "${ROBOT_USER}:${HARBOR_ROBOT_PASSWORD}" \
  "http://localhost:${HARBOR_LOCAL_PORT}/service/token?service=harbor-registry")
if [[ "${code}" == "200" ]]; then
  echo "  ${PASS} Robot   — registry token service (/service/token)"
else
  echo "  ${FAIL} Robot   — registry token service (/service/token) → HTTP ${code}"
  echo ""
  echo "  Hint: the registry token service requires a password with a special"
  echo "  character (!@#\$%^&*). If REST API passes but token service fails,"
  echo "  Harbor ignored the secret set at creation — reset it in the Harbor UI."
fi

echo ""
