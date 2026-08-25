# Envoy Proxy

```bash
$ kubectl get all -n gateway-system
NAME                                        READY   STATUS    RESTARTS   AGE
pod/shared-gateway-istio-786c8f97fb-l5twc   1/1     Running   0          3d8h

NAME                           TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)                        AGE
service/shared-gateway-istio   LoadBalancer   10.96.60.10   203.0.113.20   15021:30214/TCP,80:32114/TCP   3d11h

NAME                                   READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/shared-gateway-istio   1/1     1            1           3d11h

NAME                                              DESIRED   CURRENT   READY   AGE
replicaset.apps/shared-gateway-istio-786c8f97fb   1         1         1       3d11h
```

Port 15000 is Envoy's built-in admin interface. It's bound to localhost only (never exposed externally) and gives you raw internals of that specific Envoy process:

- `/config_dump` — full xDS config (listeners, routes, clusters, endpoints)
- `/stats` — Prometheus-style counters and gauges
- `/clusters` — upstream cluster health and connection counts
- `/listeners` — active listeners summary
- `/ready` — simple health check

The other ports on the same pod:
- `15021` — Istio readiness/liveness probe (what the Azure LB health check hits)
- `15090` — Prometheus metrics scrape endpoint
- `80` — actual traffic ingress (mapped to 32114 on the node)

`localhost:15000` only works from inside the pod via kubectl exec.

```bash
kubectl exec -n gateway-system deploy/shared-gateway-istio -c istio-proxy -- \
curl -s localhost:15000/config_dump | jq .
```

```bash
kubectl exec -n gateway-system deploy/shared-gateway-istio -c istio-proxy -- \
curl -s localhost:15000/listeners
```

```bash
kubectl exec -n gateway-system deploy/shared-gateway-istio -c istio-proxy -- \
curl -s localhost:15000/ready
```