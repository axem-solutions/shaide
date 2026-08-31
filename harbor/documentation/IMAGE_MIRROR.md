# Image Mirror

How container images are kept in sync with Harbor.

## Overview

The image-mirror component (`pkg/iac/harbor/mirror.go`) deploys automatically when both are
true:

- `harbor:robotPassword` is set (push authentication)
- `harbor:mirrorEnabled` is `true`

Setting `harbor:robotPassword` does more than gate the mirror: it's the desired password for
Harbor's `k8s-harbor-sa` robot account, which `pulumi up` creates (or updates to match) via
`pkg/iac/harbor/setup.go` — see the main README's deployment flow. The mirror simply reuses
that same account once it exists.

It manages two independent lists, each mirrored into Harbor with the same logic: compare the
source and destination digests, copy only if they differ.

## Public images

Config: `harbor:publicImages` — one `src|dest` pair per line.

Deployed as a one-shot `Job` (`harbor-image-mirror-public`), not a recurring schedule. The
list is fixed by configuration, so there is nothing to poll for — the Job only re-runs when
`harbor:publicImages` itself changes.

```bash
pulumi config set harbor:mirrorEnabled true
pulumi config set --secret harbor:robotPassword <robot-password>
pulumi config set harbor:publicImages "$(cat <<'EOF'
nginx:alpine|images-infra/nginx:alpine
alpine:3.20|images-infra/alpine:3.20
EOF
)"
```

## Private (GHCR) images

Config: `harbor:ghcrOrg`, `harbor:ghcrSyncMode`, `harbor:ghcrUser` / `harbor:ghcrToken`.

Three sync modes:

| Mode | Behavior | Deployment |
|---|---|---|
| `all` (default) | Every published tag for every package in the org, discovered via the GitHub Packages API | Recurring `CronJob`, every 5 minutes |
| `min-version` | Only tags ≥ a per-package floor (`harbor:ghcrMinVersions`) | Recurring `CronJob` |
| `pinned` | Exact `package:tag` pairs (`harbor:ghcrPinnedImages`), no API calls | One-shot `Job`, re-runs only when the list changes |

`all` and `min-version` need real polling — new tags can appear without any config change.
`pinned` is a fixed list, same reasoning as public images, so it runs as a `Job` instead.

Without `harbor:ghcrUser` and `harbor:ghcrToken`, private-image mirroring is skipped
entirely, in every mode — even for packages that happen to be public, since GitHub's
Packages API requires an authenticated token to list them regardless of visibility.

Packages not covered by the active mode's list (`pinned` or `min-version`) are skipped
entirely — there is no fallback to `all` for whatever is missing.

```bash
pulumi config set harbor:ghcrOrg axem-solutions
pulumi config set harbor:ghcrUser <ghcr-username>
pulumi config set --secret harbor:ghcrToken <ghcr-token>

# min-version: mirror every tag >= the listed floor, per package
pulumi config set harbor:ghcrSyncMode min-version
pulumi config set harbor:ghcrMinVersions "$(cat <<'EOF'
shaide_server|v0.11.0
control_panel|v0.4.0
EOF
)"

# pinned: mirror only these exact tags, no discovery call
pulumi config set harbor:ghcrSyncMode pinned
pulumi config set harbor:ghcrPinnedImages "$(cat <<'EOF'
ghcr.io/axem-solutions/shaide_server:v0.11.0|images-shaide/shaide_server:v0.11.0
EOF
)"
```

## Why a Job for fixed lists

A Kubernetes Job's pod spec is immutable once created. The image list is embedded directly
in the pod spec as an environment variable, not mounted from a ConfigMap — so editing the
underlying config value changes that spec. Pulumi then deletes and recreates the Job
(`ReplaceOnChanges`), which is exactly "re-run when the list changes." Left untouched, the
Job runs exactly once.

## Push authentication

Every mirror path authenticates to Harbor with the robot account (`harbor:robotPassword`),
regardless of whether the target project is public or private — pushing always requires
authentication; only pulling can be anonymous.

## Fix: Harbor Service port 443

Harbor runs with TLS disabled, so its Service originally had no port 443 at all. `skopeo`
(used for every copy) always attempts HTTPS first, even with `--tls-verify=false` — that flag
only skips certificate validation, not the attempt itself. With no matching Service rule, the
connection was silently dropped instead of refused, adding roughly 30-60 seconds of delay per
image, per run.

Fix: a Service patch (`fixHarborHTTPSPortBlackhole` in `pkg/iac/harbor/harbor.go`) adds port
443, routed to the same HTTP-only backend as port 80. The TLS handshake now fails fast, and
`skopeo` falls back to HTTP immediately instead of waiting out a timeout.
