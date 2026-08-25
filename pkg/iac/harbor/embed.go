package harbor

import _ "embed"

// mirrorScript is the shared skopeo digest-check/copy/failure-tracking
// implementation for all three mirror paths (public images, pinned GHCR
// images, discovered private GHCR images) — see scripts/mirror.sh.
//
//go:embed scripts/mirror.sh
var mirrorScript string

// discoverScript populates /images/private-images.txt for the private-image
// CronJob — see scripts/discover-private-images.sh.
//
//go:embed scripts/discover-private-images.sh
var discoverScript string

// nodeTrustScriptTemplate is fmt.Sprintf'd with (certsDir, harborIP) by
// deployNodeTrust — see scripts/node-trust.sh.tmpl.
//
//go:embed scripts/node-trust.sh.tmpl
var nodeTrustScriptTemplate string
