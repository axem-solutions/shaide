---
title: "Local cluster: Storage"
description: "Local development cluster - Storage."
weight: 80
---

# Local cluster: Storage

vind's `k8s` distro ships Rancher's **`local-path-provisioner`**
(`rancher.io/local-path`) out of the box — the same lightweight
provisioner k3s bundles by default. No cloud API, no CSI driver, no
NFS/iSCSI setup required; it "just works" for a single-machine dev
cluster. This is also the direct answer to `app_shaide`'s
`storageClassName` config question: leaving it unset means
shaide-server/rustfs/qdrant's PVCs all land here automatically.

## Confirming it exists and is default

```bash
$ kubectl get storageclass
NAME                   PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
local-path (default)   rancher.io/local-path   Delete          WaitForFirstConsumer   false                  12m
```

```bash
$ kubectl describe storageclass local-path
Name:            local-path
IsDefaultClass:  Yes
Annotations:     kubectl.kubernetes.io/last-applied-configuration={"apiVersion":"storage.k8s.io/v1","kind":"StorageClass","metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"},"name":"local-path"},"provisioner":"rancher.io/local-path","reclaimPolicy":"Delete","volumeBindingMode":"WaitForFirstConsumer"}
,storageclass.kubernetes.io/is-default-class=true
Provisioner:           rancher.io/local-path
Parameters:            <none>
AllowVolumeExpansion:  <unset>
MountOptions:          <none>
ReclaimPolicy:         Delete
VolumeBindingMode:     WaitForFirstConsumer
Events:                <none>
```

`IsDefaultClass: Yes` / the `(default)` marker means any PVC created
without an explicit `storageClassName` binds here automatically — no
further action needed for `app_shaide`.

## How `local-path-provisioner` actually works

There's no real storage backend behind it — "provisioning" a volume means
the provisioner runs `mkdir` for a new directory on the **node's local
disk**, then hands that back to Kubernetes as a `hostPath` volume.

That's exactly why `VOLUMEBINDINGMODE` is `WaitForFirstConsumer`: the
provisioner can't create the directory until it knows *which node* the
Pod using the PVC will land on. Verified on a live cluster:

```bash
$ kubectl apply -f - <<'EOF'
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: storage-demo
spec:
  accessModes: ["ReadWriteOnce"]
  resources:
    requests:
      storage: 1Gi
EOF

$ kubectl get pvc storage-demo
NAME           STATUS    VOLUME   CAPACITY   ACCESS MODES   STORAGECLASS   AGE
storage-demo   Pending                                      local-path     0s
```

Still `Pending` — no Pod has claimed it yet, so the provisioner has
nothing to schedule against. Creating a Pod that mounts it triggers
binding:

```bash
$ kubectl get pvc storage-demo
NAME           STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
storage-demo   Bound    pvc-f54f0db2-6535-4626-a69e-1318d00adf92   1Gi        RWO            local-path     6s

$ kubectl get pod storage-demo-pod -o wide
NAME               READY   STATUS              RESTARTS   AGE   NODE
storage-demo-pod   0/1     ContainerCreating   0          6s    worker-1
```

Only once the scheduler picked `worker-1` for the Pod did the PV actually
get created — and it gets a `nodeAffinity` baked in pinning it to
`worker-1` specifically. The data isn't replicated or network-accessible;
it lives on that one node's local disk, and any future Pod reusing this
PVC is forced back onto the same node.

- **`RECLAIMPOLICY: Delete`** — deleting the PVC also deletes the backing
  directory; no data survives past the claim's lifecycle. Contrast with
  the static hostPath pattern in [`VOLUMES.md`](volumes.md), which
  deliberately uses `Retain`.
- **`ALLOWVOLUMEEXPANSION: false`** — can't grow a PVC in place; delete
  and recreate it larger instead.

## Known limitations (from real-world reports, not vind-specific)

Rancher's `local-path-provisioner` has well-documented sharp edges,
covered in
[Addressing the Limitations of Local-Path Provisioner in Kubernetes](https://dev.to/frosnerd/addressing-the-limitations-of-local-path-provisioner-in-kubernetes-3g12).
The node-pinning behavior above is the root cause of all three:

- **Pods can become permanently unschedulable.** If the node a PVC's data
  lives on becomes unavailable, a Pod that gets recreated is *still*
  bound to that exact node via the PV's `nodeAffinity` — if the node is
  gone, nothing can satisfy the constraint, and the Pod sits `Pending`
  until the PVC is deleted manually. **This is a real risk on vind
  specifically**: the `docker rm -f vcluster.node.<cluster>.<worker>` +
  `--upgrade` recreation flow documented in
  [`TROUBLESHOOTING.md`](troubleshooting.md) replaces a worker container
  with a fresh one — any `local-path` PV that lived on the old container
  is now orphaned, and any Pod claiming it will be stuck.
- **Orphaned PVs.** If the node is gone, the provisioner's cleanup helper
  pod can't be scheduled there either, so deleting the PVC doesn't
  actually remove the backing directory — the PV object can get stuck
  needing manual `kubectl delete pv --force` cleanup.
- **No capacity enforcement.** A PVC's `storage: 1Gi` request isn't an
  enforced quota — `local-path-provisioner` just creates a directory with
  no size limit. Multiple PVCs can silently overcommit the same disk,
  producing out-of-disk errors that don't look like the expected
  "PVC full" error. Relevant for `app_shaide`'s
  `shaideServerPVSize`/`rustfsPVSize`/`qdrantPVSize` config keys — those
  sizes aren't actually enforced on this cluster, just advisory.

None of this matters much for disposable local-dev use (delete/recreate
the whole cluster if storage gets into a bad state — see README.md step
7), but worth knowing before relying on it for anything you'd mind losing.

## Cleanup (if you ran the worked example above)

```bash
kubectl delete pod storage-demo-pod
kubectl delete pvc storage-demo
```
