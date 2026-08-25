#!/usr/bin/env bash
# Deploy a minimal nginx pod + HTTPRoute to verify the shared gateway is working.
# Tests: AGC provisioning, route programming, HTTP/HTTPS connectivity.
#
# Usage:
#   ./verify-gateway.sh              # deploy + test
#   ./verify-gateway.sh status       # check infrastructure status only (no deploy)
#   ./verify-gateway.sh cleanup      # remove test resources
set -euo pipefail

NAMESPACE="test-gateway"
GATEWAY_NAME="shared-gateway"
GATEWAY_NS="gateway-system"
ALB_NAME="shared-alb"

if [[ -t 1 ]]; then
    GREEN='\033[0;32m' YELLOW='\033[1;33m' RED='\033[0;31m' CYAN='\033[0;36m' DIM='\033[2m' RESET='\033[0m'
else
    GREEN='' YELLOW='' RED='' CYAN='' DIM='' RESET=''
fi

log()  { echo -e "${GREEN}▸${RESET} $*"; }
warn() { echo -e "${YELLOW}▸${RESET} $*"; }
fail() { echo -e "${RED}✗${RESET} $*"; }
ok()   { echo -e "${GREEN}✓${RESET} $*"; }
info() { echo -e "${CYAN}ℹ${RESET} $*"; }

# ─── cleanup ─────────────────────────────────────────────────────────────────
if [[ "${1:-}" == "cleanup" ]]; then
    log "Deleting namespace $NAMESPACE..."
    kubectl delete namespace "$NAMESPACE" --ignore-not-found
    log "Done."
    exit 0
fi

# ─── infrastructure status ───────────────────────────────────────────────────
check_infra() {
    echo ""
    log "=== Infrastructure Status ==="
    echo ""

    # ALB Controller pods
    log "ALB Controller pods (kube-system):"
    ALB_PODS=$(kubectl get pods -n kube-system -l app=alb-controller --no-headers 2>/dev/null || true)
    if [[ -n "$ALB_PODS" ]]; then
        RUNNING=$(echo "$ALB_PODS" | grep -c Running || true)
        TOTAL=$(echo "$ALB_PODS" | wc -l)
        if [[ "$RUNNING" -eq "$TOTAL" ]]; then
            ok "  $RUNNING/$TOTAL pods Running"
        else
            warn "  $RUNNING/$TOTAL pods Running"
            echo "$ALB_PODS" | sed 's/^/    /'
        fi
    else
        fail "  No ALB Controller pods found"
    fi

    # GatewayClass
    log "GatewayClass:"
    if kubectl get gatewayclass azure-alb-external --no-headers &>/dev/null; then
        ok "  azure-alb-external present"
    else
        fail "  azure-alb-external NOT found — ALB Controller may not be ready"
    fi

    # ApplicationLoadBalancer
    log "ApplicationLoadBalancer ($GATEWAY_NS/$ALB_NAME):"
    ALB_EXISTS=$(kubectl get applicationloadbalancer "$ALB_NAME" -n "$GATEWAY_NS" --no-headers 2>/dev/null || true)
    if [[ -z "$ALB_EXISTS" ]]; then
        fail "  Not found"
    else
        ALB_ACCEPTED=$(kubectl get applicationloadbalancer "$ALB_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.conditions[?(@.type=="Accepted")].status}' 2>/dev/null || true)
        ALB_DEPLOYED=$(kubectl get applicationloadbalancer "$ALB_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.conditions[?(@.type=="Deployment")].status}' 2>/dev/null || true)
        ALB_DEPLOY_MSG=$(kubectl get applicationloadbalancer "$ALB_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.conditions[?(@.type=="Deployment")].message}' 2>/dev/null || true)

        if [[ "$ALB_ACCEPTED" == "True" ]]; then
            ok "  Accepted: True"
        else
            warn "  Accepted: ${ALB_ACCEPTED:-pending}"
        fi

        if [[ "$ALB_DEPLOYED" == "True" ]]; then
            ok "  Deployment: Ready"
        elif [[ -z "$ALB_DEPLOYED" ]]; then
            warn "  Deployment: pending — AGC provisioning takes 5-6 minutes"
        else
            fail "  Deployment: $ALB_DEPLOYED"
            if [[ -n "$ALB_DEPLOY_MSG" ]]; then
                echo "$ALB_DEPLOY_MSG" | head -5 | sed 's/^/    /'
            fi
        fi

        # Check Azure-side association status
        MC_RG=$(az aks show --resource-group "$(kubectl config current-context | sed 's/aks-/rg-ai-platform-/')" --name "$(kubectl config current-context)" --query nodeResourceGroup -o tsv 2>/dev/null || true)
        if [[ -n "$MC_RG" ]]; then
            ALB_AZURE_NAME=$(az network alb list --resource-group "$MC_RG" --query "[0].name" -o tsv 2>/dev/null || true)
            if [[ -n "$ALB_AZURE_NAME" ]]; then
                ASSOC_STATE=$(az network alb association list --alb-name "$ALB_AZURE_NAME" --resource-group "$MC_RG" --query "[0].provisioningState" -o tsv 2>/dev/null || true)
                if [[ "$ASSOC_STATE" == "Succeeded" ]]; then
                    ok "  Azure association: Succeeded"
                elif [[ -n "$ASSOC_STATE" ]]; then
                    warn "  Azure association: $ASSOC_STATE"
                fi
            fi
        fi
    fi

    # Gateway
    log "Gateway ($GATEWAY_NS/$GATEWAY_NAME):"
    GW_EXISTS=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" --no-headers 2>/dev/null || true)
    if [[ -z "$GW_EXISTS" ]]; then
        fail "  Not found"
    else
        GW_ACCEPTED=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.conditions[?(@.type=="Accepted")].status}' 2>/dev/null || true)
        GW_PROGRAMMED=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.conditions[?(@.type=="Programmed")].status}' 2>/dev/null || true)
        GW_IP=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)

        if [[ "$GW_ACCEPTED" == "True" ]]; then
            ok "  Accepted: True"
        else
            warn "  Accepted: ${GW_ACCEPTED:-pending}"
        fi

        if [[ "$GW_PROGRAMMED" == "True" ]]; then
            ok "  Programmed: True"
        else
            warn "  Programmed: ${GW_PROGRAMMED:-pending}"
        fi

        GW_ADDR_TYPE=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.addresses[0].type}' 2>/dev/null || true)
        GW_ADDR_VALUE=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
        if [[ -n "$GW_ADDR_VALUE" ]]; then
            ok "  Address: $GW_ADDR_VALUE (${GW_ADDR_TYPE:-unknown})"
            if [[ "$GW_ADDR_TYPE" == "Hostname" ]]; then
                GW_RESOLVED_IP=$(dig +short "$GW_ADDR_VALUE" 2>/dev/null | head -1 || true)
                if [[ -n "$GW_RESOLVED_IP" ]]; then
                    info "  Resolves to: $GW_RESOLVED_IP"
                fi
                info "  DNS: create CNAME record pointing your domain → $GW_ADDR_VALUE"
            fi
        else
            warn "  Address: not assigned yet"
        fi

        # Listeners
        LISTENERS=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" -o jsonpath='{range .spec.listeners[*]}{.name}:{.protocol}:{.port} {end}' 2>/dev/null || true)
        if [[ -n "$LISTENERS" ]]; then
            info "  Listeners: $LISTENERS"
        fi
    fi

    # cert-manager (if deployed)
    if kubectl get namespace cert-manager &>/dev/null; then
        log "cert-manager:"
        CM_PODS=$(kubectl get pods -n cert-manager --no-headers 2>/dev/null | grep -c Running || true)
        CM_TOTAL=$(kubectl get pods -n cert-manager --no-headers 2>/dev/null | wc -l)
        if [[ "$CM_PODS" -eq "$CM_TOTAL" && "$CM_TOTAL" -gt 0 ]]; then
            ok "  $CM_PODS/$CM_TOTAL pods Running"
        else
            warn "  $CM_PODS/$CM_TOTAL pods Running"
        fi

        ISSUER_READY=$(kubectl get clusterissuer letsencrypt-azure -o jsonpath='{.status.conditions[0].status}' 2>/dev/null || true)
        if [[ "$ISSUER_READY" == "True" ]]; then
            ok "  ClusterIssuer letsencrypt-azure: Ready"
        elif [[ -n "$ISSUER_READY" ]]; then
            warn "  ClusterIssuer letsencrypt-azure: $ISSUER_READY"
        else
            info "  ClusterIssuer letsencrypt-azure: not found"
        fi

        CERT_READY=$(kubectl get certificate gateway-tls-cert -n "$GATEWAY_NS" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || true)
        if [[ "$CERT_READY" == "True" ]]; then
            ok "  Certificate gateway-tls-cert: Ready"
        elif [[ -n "$CERT_READY" ]]; then
            warn "  Certificate gateway-tls-cert: $CERT_READY"
        else
            info "  Certificate gateway-tls-cert: not created (TLS not enabled yet)"
        fi

        TLS_SECRET=$(kubectl get secret gateway-tls -n "$GATEWAY_NS" --no-headers 2>/dev/null || true)
        if [[ -n "$TLS_SECRET" ]]; then
            ok "  Secret gateway-tls: exists"
        else
            info "  Secret gateway-tls: not found (expected if TLS not enabled)"
        fi
    fi

    # ALB Controller recent errors
    ALB_ERRORS=$(kubectl logs -n kube-system -l app=alb-controller --tail=50 2>/dev/null | grep -i '"level":"error"' | tail -3 || true)
    if [[ -n "$ALB_ERRORS" ]]; then
        echo ""
        warn "Recent ALB Controller errors:"
        echo "$ALB_ERRORS" | while read -r line; do
            MSG=$(echo "$line" | grep -o '"message":"[^"]*"' | head -1 || true)
            echo -e "    ${DIM}${MSG}${RESET}"
        done
    fi
}

# ─── status only ─────────────────────────────────────────────────────────────
if [[ "${1:-}" == "status" ]]; then
    check_infra
    exit 0
fi

# ─── deploy ──────────────────────────────────────────────────────────────────
check_infra

echo ""
log "=== Deploying test workload ==="
echo ""

log "Creating namespace $NAMESPACE..."
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

log "Deploying nginx + Service + HTTPRoute..."
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: $NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: nginx
  namespace: $NAMESPACE
spec:
  selector:
    app: nginx
  ports:
  - port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: nginx-route
  namespace: $NAMESPACE
spec:
  parentRefs:
  - name: $GATEWAY_NAME
    namespace: $GATEWAY_NS
  rules:
  - backendRefs:
    - name: nginx
      port: 80
EOF

log "Waiting for nginx pod to be ready..."
kubectl wait --for=condition=ready pod -l app=nginx -n "$NAMESPACE" --timeout=60s

# ─── HTTPRoute status ────────────────────────────────────────────────────────
echo ""
log "=== HTTPRoute Status ==="
echo ""
ROUTE_ACCEPTED=$(kubectl get httproute nginx-route -n "$NAMESPACE" -o jsonpath='{.status.parents[0].conditions[?(@.type=="Accepted")].status}' 2>/dev/null || true)
ROUTE_PROGRAMMED=$(kubectl get httproute nginx-route -n "$NAMESPACE" -o jsonpath='{.status.parents[0].conditions[?(@.type=="Programmed")].status}' 2>/dev/null || true)

if [[ "$ROUTE_ACCEPTED" == "True" ]]; then
    ok "Accepted: True"
else
    warn "Accepted: ${ROUTE_ACCEPTED:-pending} — AGC may still be programming"
fi
if [[ "$ROUTE_PROGRAMMED" == "True" ]]; then
    ok "Programmed: True"
else
    warn "Programmed: ${ROUTE_PROGRAMMED:-pending}"
fi

# ─── connectivity tests ─────────────────────────────────────────────────────
echo ""
log "=== Connectivity Tests ==="
echo ""

GW_ADDR_TYPE=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.addresses[0].type}' 2>/dev/null || true)
GW_ADDR=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || true)
GW_LISTENER_HOSTNAME=$(kubectl get gateway "$GATEWAY_NAME" -n "$GATEWAY_NS" -o jsonpath='{.spec.listeners[0].hostname}' 2>/dev/null || true)

if [[ -z "$GW_ADDR" ]]; then
    warn "No gateway address — skipping connectivity tests"
    warn "Run '$0 status' to check infrastructure readiness"
else
    # AGC uses a Hostname address; GCP/on-prem use an IP.
    # For AGC, test via the hostname with the listener hostname as Host header.
    if [[ "$GW_ADDR_TYPE" == "Hostname" ]]; then
        log "Testing HTTP via AGC hostname ($GW_ADDR)..."
        if [[ -n "$GW_LISTENER_HOSTNAME" ]]; then
            HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 --max-time 15 -H "Host: $GW_LISTENER_HOSTNAME" "http://$GW_ADDR/" 2>/dev/null || echo "000")
        else
            HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 --max-time 15 "http://$GW_ADDR/" 2>/dev/null || echo "000")
        fi
    else
        log "Testing HTTP via IP ($GW_ADDR)..."
        HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 --max-time 15 "http://$GW_ADDR/" 2>/dev/null || echo "000")
    fi

    if [[ "$HTTP_STATUS" == "200" ]]; then
        ok "  HTTP $HTTP_STATUS"
    elif [[ "$HTTP_STATUS" == "404" ]]; then
        warn "  HTTP $HTTP_STATUS — route not matched (hostname mismatch or HTTPRoute not programmed)"
    elif [[ "$HTTP_STATUS" == "000" ]]; then
        fail "  Connection timeout — AGC may still be provisioning"
        info "  Run '$0 status' to check ApplicationLoadBalancer Deployment condition"
    else
        warn "  HTTP $HTTP_STATUS"
    fi

    # Test via the configured domain (requires DNS CNAME → AGC hostname)
    if [[ -n "$GW_LISTENER_HOSTNAME" ]]; then
        echo ""
        log "Testing HTTP via domain ($GW_LISTENER_HOSTNAME)..."
        HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 --max-time 15 "http://$GW_LISTENER_HOSTNAME/" 2>/dev/null || echo "000")
        if [[ "$HTTP_STATUS" == "200" ]]; then
            ok "  HTTP $HTTP_STATUS"
        elif [[ "$HTTP_STATUS" == "000" ]]; then
            fail "  Connection timeout — DNS may not be configured yet"
            if [[ "$GW_ADDR_TYPE" == "Hostname" ]]; then
                info "  Create CNAME: $GW_LISTENER_HOSTNAME → $GW_ADDR"
            fi
        else
            warn "  HTTP $HTTP_STATUS"
        fi

        echo ""
        log "Testing HTTPS via domain ($GW_LISTENER_HOSTNAME)..."
        HTTPS_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 --max-time 15 -k "https://$GW_LISTENER_HOSTNAME/" 2>/dev/null || echo "000")
        if [[ "$HTTPS_STATUS" == "200" ]]; then
            ok "  HTTPS $HTTPS_STATUS"
            ISSUER=$(curl -s --connect-timeout 10 -k -v "https://$GW_LISTENER_HOSTNAME/" 2>&1 | grep "issuer:" | head -1 || true)
            if [[ -n "$ISSUER" ]]; then
                ok "  $ISSUER"
            fi
        elif [[ "$HTTPS_STATUS" == "000" ]]; then
            info "  HTTPS not available — TLS may not be enabled or cert not issued yet"
        else
            warn "  HTTPS $HTTPS_STATUS"
        fi
    fi
fi

echo ""
log "Cleanup when done: $0 cleanup"
