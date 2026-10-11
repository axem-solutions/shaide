---
title: "Storage and volumes"
description: "Resizing persistent volumes and planning storage capacity."
weight: 60
---

# Storage and volumes

This procedure applies to any Kubernetes cluster where `app_shaide` is deployed.

Kubernetes does not allow mutating `volumeClaimTemplates` on an existing StatefulSet, so the
StatefulSet must be deleted and recreated as part of this procedure. The PVC has an independent
lifecycle and survives StatefulSet deletion — data is not lost.

## Prerequisites

The StorageClass used by the PVC must have `allowVolumeExpansion: true`. Verify before proceeding:

```bash
kubectl get storageclass -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.allowVolumeExpansion}{"\n"}{end}'
```

Proceed only if the relevant StorageClass shows `true`.

## Procedure

**1. Select the correct cluster context**

```bash
kubectl config get-contexts
kubectl config use-context <context_name>
```

**2. Patch the PVC to request the new size**

```bash
kubectl patch pvc <pvc_name> -n <namespace> \
  -p '{"spec":{"resources":{"requests":{"storage":"<new_size>"}}}}'
```

**3. Wait for the resize to complete**

```bash
kubectl get pvc <pvc_name> -n <namespace> -w
```

Wait until the `CAPACITY` column reflects the new size.

**4. Delete the StatefulSet, keeping the PVC**

The `--cascade=orphan` flag removes the StatefulSet object while leaving the PVC and its data intact.

```bash
kubectl delete statefulset shaide-server -n <namespace> --cascade=orphan
```

**5. Reconcile with Pulumi**

```bash
pulumi up
```

Pulumi recreates the StatefulSet. It may show a `replace` for the StatefulSet — this is expected.
The PVC is reattached and data is preserved.

**6. Verify**

```bash
kubectl get pvc <pvc_name> -n <namespace>
```

Confirm the `CAPACITY` column reflects the new size.
