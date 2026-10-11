---
title: "Local cluster: Networking"
description: "Local development cluster - Networking."
weight: 50
---

# Local cluster: Networking

```yaml
networking:
  podCIDR: 10.64.0.0/16
  serviceCIDR: 10.128.0.0/16
```

This is set in `cluster.yaml` to avoid conflicts between the cluster's
default ranges and other networks a developer's laptop might be on (VPN,
home network, etc).

## What these fields mean

- **`podCIDR`** — the address range Pod IPs are allocated from (managed by
  the CNI; flannel in this cluster). Stock default: `10.244.0.0/16`.
- **`serviceCIDR`** — the address range `ClusterIP` Services are allocated
  from. Stock default: `10.96.0.0/12` (this is where the `kubernetes`
  Service's well-known `10.96.0.1` comes from on an unconfigured cluster).

## Docs say this is for Private Nodes — testing showed it works standalone too

vCluster's own schema description scopes this field to a different
feature: *"This should only be set if `privateNodes.enabled` is true or
vCluster cannot detect the host service cidr"* — Private Nodes being a
separate tenancy model (external bare-metal/cloud nodes joining a vCluster
control plane via Konnectivity tunneling), architecturally unrelated to
vind's standalone Docker-container nodes.

That description turned out to be overly narrow. **Confirmed by directly
testing against a live vind cluster** (no `privateNodes` involved at all):
adding the block above, then `vcluster delete local-k8s && vcluster
create local-k8s --values cluster.yaml`, produced:

```bash
$ kubectl get svc kubernetes
NAME         TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)   AGE
kubernetes   ClusterIP   10.128.0.1   <none>        443/TCP   42s
```

`10.128.0.1` — inside the configured `serviceCIDR`, not the stock
`10.96.0.0/12` range.

```bash
$ kubectl -n kube-flannel get cm kube-flannel-cfg -o yaml | grep -A3 -i network
      "Network": "10.64.0.0/16",
      "EnableNFTables": false,
      "Backend": {
        "Type": "vxlan"
```

flannel's own config picked up the configured `podCIDR` exactly.

```bash
$ kubectl get pod -A -o wide --no-headers | grep coredns
kube-system   coredns-df8c87f55-27dnq   1/1   Running   0   33s   10.64.0.3   local-k8s   <none>   <none>
```

And a real Pod (CoreDNS) actually got an IP inside that range —
confirming it's not just a reported config value, Pods genuinely get
addresses from it. (`kube-flannel-ds` pods themselves show Docker-bridge
IPs like `172.18.0.x` in the same listing — expected, since they run with
`hostNetwork: true` and don't get a CNI-assigned address.)

## Caveat: requires full cluster recreation

Like most cluster-bring-up config, this is baked in when the control
plane and CNI first initialize — there's no live way to move an existing
cluster's Pods/Services to a new CIDR range. Changing `podCIDR`/
`serviceCIDR` in `cluster.yaml` requires the full recreate flow (see
README.md step 7), not `--upgrade`:

```bash
vcluster delete local-k8s
vcluster create local-k8s --values cluster.yaml
```

## Picking ranges

Choose ranges that don't overlap your host machine's own network, VPN
ranges, or (if relevant) any other vind clusters you're running
concurrently (see README.md step 6, "Multiple clusters from one file") —
each vind cluster is a fully separate Docker network, so overlapping
CIDRs between two *different* clusters isn't itself a conflict, but
overlapping with whatever network your laptop is actually routing through
(corporate VPN being the common case) will cause routing problems for
anything inside the cluster trying to reach addresses in that overlapping
range.
