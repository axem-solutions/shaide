---
title: "Local cluster: Lb"
description: "Local development cluster - Lb."
weight: 30
---

# Local cluster: Lb

vind ships LoadBalancer support out of the box — no MetalLB, no cloud
provider integration needed. Creating a `Service` of type `LoadBalancer`
makes vind spin up a **dedicated HAProxy container** that gets its own IP
on the cluster's Docker bridge network and forwards traffic to the
Service's backing pods. This page walks through a live test of that path,
end to end, with an explanation of what's actually happening at each step.

## Reference

[Getting Started with vind: First Kubernetes Deployment with Built-in LoadBalancer](https://www.vcluster.com/blog/vind-getting-started-first-deployment-loadbalancer-kubernetes-docker)

## 1. Baseline — nothing but cluster-internal Services yet

```bash
$ kubectl get svc -A
NAMESPACE     NAME         TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)                  AGE
default       kubernetes   ClusterIP   10.96.0.1        <none>        443/TCP                  86m
kube-system   kube-dns     ClusterIP   10.108.231.123   <none>        53/UDP,53/TCP,9153/TCP   86m

$ kubectl config current-context
vcluster-docker_local-k8s
```

Only the two default `ClusterIP` services exist — expected, since nothing
has requested a `LoadBalancer` yet.

## 2. Create a test deployment and expose it

```bash
$ kubectl create deployment nginx-test --image=nginx
deployment.apps/nginx-test created

$ kubectl get pod
NAME                         READY   STATUS    RESTARTS   AGE
nginx-test-c8697b5cc-wrmfh   1/1     Running   0          111s

$ kubectl get svc nginx-test
NAME         TYPE           CLUSTER-IP      EXTERNAL-IP      PORT(S)        AGE
nginx-test   LoadBalancer   10.104.183.62   172.18.255.254   80:31681/TCP   14s
```

`EXTERNAL-IP` populates almost immediately — no `<pending>` state, unlike
a real cloud cluster waiting on a cloud LB controller. `172.18.255.254` is
not a real external/routable address; it's an IP on vind's own Docker
bridge network for this cluster (confirmed in step 4 below). The
`80:31681/TCP` also shows the usual `NodePort` (`31681`) got allocated
underneath, same as any `LoadBalancer` Service — vind's HAProxy container
is what actually answers on `172.18.255.254:80` and forwards to it.

## 3. Confirm it's actually reachable

```bash
$ kubectl get svc -A
NAMESPACE     NAME         TYPE           CLUSTER-IP       EXTERNAL-IP      PORT(S)                  AGE
default       kubernetes   ClusterIP      10.96.0.1        <none>           443/TCP                  99m
default       nginx-test   LoadBalancer   10.104.183.62    172.18.255.254   80:31681/TCP             4m32s
kube-system   kube-dns     ClusterIP      10.108.231.123   <none>           53/UDP,53/TCP,9153/TCP   99m

$ curl -v 172.18.255.254
*   Trying 172.18.255.254:80...
* Connected to 172.18.255.254 (172.18.255.254) port 80
> GET / HTTP/1.1
> Host: 172.18.255.254
> User-Agent: curl/8.5.0
> Accept: */*
>
< HTTP/1.1 200 OK
< Server: nginx/1.31.2
...
* Connection #0 to host 172.18.255.254 left intact
```

This is the actual proof: `curl` from the **host laptop**, straight to the
`EXTERNAL-IP`, no `kubectl port-forward`, no `minikube tunnel` equivalent,
no manual step. That only works because the Docker bridge is directly
routable from a Linux host — see the platform caveat at the bottom.

## 4. What made this work — the HAProxy container

```bash
$ docker ps
CONTAINER ID   IMAGE                          COMMAND                  CREATED          STATUS          PORTS                                           NAMES
10aa96f4d791   haproxy:3.3-alpine             "docker-entrypoint.s…"   12 seconds ago   Up 11 seconds   80/tcp                                          vcluster.lb.local-k8s.nginx-test.default
96a1075b352e   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"         2 hours ago      Up 2 hours                                                      vcluster.node.local-k8s.worker-3
f3cba5829f44   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"         2 hours ago      Up 2 hours                                                      vcluster.node.local-k8s.worker-2
61e7dd1a8ed6   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"         2 hours ago      Up 2 hours                                                      vcluster.node.local-k8s.worker-1
34dea50e8187   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"         2 hours ago      Up 2 hours      0.0.0.0:10354->8443/tcp, [::]:10354->8443/tcp   vcluster.cp.local-k8s
```

Creating the `LoadBalancer` Service made vind start a **new container**,
`vcluster.lb.local-k8s.nginx-test.default` — naming follows
`vcluster.lb.<cluster-name>.<service-name>.<namespace>`, so it's one
HAProxy container per `LoadBalancer` Service, deleted automatically when
the Service is deleted.

Note its `PORTS` column: `80/tcp`, with no `0.0.0.0:...->` host-port
mapping like `vcluster.cp.local-k8s` has. It isn't reachable because
Docker published a port to the host (like `experimental.docker.ports`,
see [`PORTS.md`](ports.md)) — it's reachable because it has its own IP
directly on the Docker bridge network, which the host can route to
without any port mapping at all.

## 5. Where `172.18.255.254` actually comes from

```bash
$ ip a s
...
7: br-cb43565b1f64: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default
    link/ether ba:a8:0d:1d:31:3b brd ff:ff:ff:ff:ff:ff
    inet 172.18.0.1/16 brd 172.18.255.255 scope global br-cb43565b1f64
       valid_lft forever preferred_lft forever
...
```

`br-cb43565b1f64` is the Docker bridge network vind created for this
cluster (`vcluster.local-k8s`, see README.md's "Docker container mapping"
section) — its subnet is `172.18.0.0/16`. `172.18.255.254`, the Service's
`EXTERNAL-IP`, is an address on that same `/16`, assigned to the HAProxy
container. Since the bridge's gateway (`172.18.0.1`) is a real interface
on the host (visible in `ip a s` output as `br-cb43565b1f64`), the Linux
kernel routes host-originated traffic to any IP in that `/16` straight to
the matching container — no port-forwarding, no NAT rule needed, which is
exactly why `curl 172.18.255.254` in step 3 worked directly from the host.

## How it all fits together

```
kubectl apply (Service type=LoadBalancer)
        │
        ▼
vind creates vcluster.lb.<cluster>.<service>.<namespace>  (haproxy:3.3-alpine)
        │  joined to the cluster's own Docker bridge (br-...)
        │  gets an IP from that bridge's subnet — becomes EXTERNAL-IP
        ▼
HAProxy forwards → Service's NodePort (allocated automatically, e.g. 31681)
        ▼
kube-proxy / CNI routes → the actual backing Pod(s)
```

## Cleanup

```bash
kubectl delete deployment nginx-test
kubectl delete svc nginx-test
docker ps --filter name=vcluster.lb.local-k8s.nginx-test   # confirm the HAProxy container is gone
```

## Platform caveat

This whole flow — automatic `EXTERNAL-IP`, directly `curl`-able from the
host — is **Linux-specific**, because it relies on the Docker bridge
network being a real, host-routable interface (`br-cb43565b1f64` above).
On macOS, Docker runs inside a Linux VM (Docker Desktop/Rancher Desktop),
so the bridge IP isn't reachable from the actual macOS host — you'd need
`sudo vcluster create` for full HAProxy support, or fall back to
`experimental.docker.loadBalancer.forwardPorts` / NodePort. See
[`PORTS.md`](ports.md) for the related `forwardPorts` option.
