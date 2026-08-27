---
title: "Local cluster: Troubleshooting"
description: "Local development cluster - Troubleshooting."
weight: 90
---

# Local cluster: Troubleshooting

## Pods stuck in `ContainerCreating` — flannel `subnet.env: no such file or directory`

```
Warning  FailedCreatePodSandBox  34s   kubelet  Failed to create pod sandbox: rpc error: code = Unknown desc = failed to setup network for sandbox "...": plugin type="flannel" failed (add): loadFlannelSubnetEnv failed: open /run/flannel/subnet.env: no such file or directory
```

**Cause:** the `br_netfilter` kernel module isn't loaded on the Docker host.
This is the same condition `vcluster create` warns about at cluster-creation
time (`Could not load kernel module br_netfilter: exit status 1`) — flannel
needs it to finish setting up pod networking, so `/run/flannel/subnet.env`
never gets written and every pod sandbox fails.

**Fix:**

```bash
sudo modprobe overlay
sudo modprobe bridge
sudo modprobe br_netfilter

# verify it took:
lsmod | grep br_netfilter
cat /proc/sys/net/bridge/bridge-nf-call-iptables   # should now exist (0 or 1), not error
```

Then let the stuck pod retry:

```bash
kubectl delete pod -l app=nginx-test
kubectl get pod -l app=nginx-test -w
```

If it still fails after the module loads, restart the flannel DaemonSet so
it notices the newly-available module:

```bash
kubectl -n kube-flannel rollout restart daemonset kube-flannel-ds
```

**Make it permanent:** `br_netfilter` isn't loaded by default on most
distros and doesn't survive a reboot unless configured to auto-load. Add it
to `/etc/modules-load.d/` so every fresh laptop setup has it loaded before
`vcluster create` runs:

```bash
echo -e "overlay\nbridge\nbr_netfilter" | sudo tee /etc/modules-load.d/vind.conf
```

## Added/changed `env` (or `volumes`) in `cluster.yaml` but it's not showing up on an existing node

```bash
$ docker exec vcluster.node.local-k8s.worker-1 env | grep NODE_ROLE
# (no output)
```

**Cause:** this is a **Docker-level** constraint, not a vcluster bug.
Container env vars (and bind mounts) are fixed at `docker
create`/`docker run` time — they can't be injected into a container that's
already running. `vcluster create --values cluster.yaml --upgrade` only
creates node containers that don't exist yet; it does not recreate
already-existing ones to apply config changes. See
[`documentation/ENV.md`](ENV.md) for the full writeup and confirmation
against a live cluster (a genuinely *new* node picks up its `env:`
correctly, since it's created fresh — see README.md step 5, "Adding a
node" — but an already-existing one does not).

**Fix:** force the specific node's container to be recreated, or recreate
the whole cluster:

```bash
# targeted (unverified — may leave a stale Node object needing
# `kubectl delete node <name>` if it doesn't cleanly re-register):
docker rm -f vcluster.node.local-k8s.worker-1
vcluster create local-k8s --values cluster.yaml --upgrade

# guaranteed (recreates every container from the current cluster.yaml):
vcluster delete local-k8s
vcluster create local-k8s --values cluster.yaml
```

**Example:**
```bash
$ docker exec vcluster.node.local-k8s.worker-1 env | grep NODE_ROLE

$ vcluster delete local-k8s
12:38:37 info Removing vCluster container vcluster.cp.local-k8s...
12:38:40 info Removing vCluster node worker-3...
12:38:42 info Removing vCluster node worker-2...
12:38:44 info Removing vCluster node worker-1...
12:38:45 info Removing vCluster load balancer nginx-test.default...
12:38:47 info Deleted kube context vcluster-docker_local-k8s
12:38:47 done Successfully deleted virtual cluster local-k8s

$ vcluster create local-k8s --values cluster.yaml
12:39:05 warn There is a newer version of vcluster: v0.35.2. Run `vcluster upgrade` to upgrade to the newest version.

12:39:05 info Ensuring environment for vCluster local-k8s...
12:39:06 done Created network vcluster.local-k8s
12:39:08 info Starting vCluster standalone local-k8s
12:39:10 info Waiting for vCluster standalone node to be joined...
12:39:27 done vCluster standalone node joined successfully
12:39:27 info Adding node worker-1 to vCluster local-k8s
12:39:28 info Joining node vcluster.node.local-k8s.worker-1 to vCluster local-k8s...
12:39:33 info Adding node worker-2 to vCluster local-k8s
12:39:34 info Joining node vcluster.node.local-k8s.worker-2 to vCluster local-k8s...
12:39:40 info Adding node worker-3 to vCluster local-k8s
12:39:40 info Joining node vcluster.node.local-k8s.worker-3 to vCluster local-k8s...
12:39:47 done Successfully created virtual cluster local-k8s
12:39:47 info Finding docker container vcluster.cp.local-k8s...
12:39:47 info Waiting for vCluster kubeconfig to be available...
12:39:47 info Waiting for vCluster to become ready...
12:39:47 done vCluster is ready
12:39:47 done Switched active kube context to vcluster-docker_local-k8s
- Use `vcluster disconnect` to return to your previous kube context
- Use `kubectl get namespaces` to access the vcluster

$ kubectl get node
NAME        STATUS   ROLES                  AGE   VERSION
local-k8s   Ready    control-plane,master   36s   v1.36.0
worker-1    Ready    <none>                 30s   v1.36.0
worker-2    Ready    <none>                 24s   v1.36.0
worker-3    Ready    <none>                 17s   v1.36.0

$ docker ps
CONTAINER ID   IMAGE                          COMMAND            CREATED              STATUS              PORTS                                           NAMES
77ae671e2c63   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"   30 seconds ago       Up 29 seconds                                                       vcluster.node.local-k8s.worker-3
61d5ad42e512   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"   37 seconds ago       Up 36 seconds                                                       vcluster.node.local-k8s.worker-2
105996691a92   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"   43 seconds ago       Up 42 seconds                                                       vcluster.node.local-k8s.worker-1
e10468186cb7   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"   About a minute ago   Up About a minute   0.0.0.0:10140->8443/tcp, [::]:10140->8443/tcp   vcluster.cp.local-k8s

$ docker exec vcluster.node.local-k8s.worker-1 env | grep NODE_ROLE
NODE_ROLE=worker
```
