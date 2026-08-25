package harbor

// Resource and object names used across this package, kept in one place so
// setup, mirroring, secrets, and node trust all reference the same values
// instead of repeating string literals that can drift out of sync (e.g. the
// mirror-creds Secret name used to be typed out separately in both the
// Secret's own metadata and in every secretEnvVar call reading from it).
const (
	// harborNamespaceDefault is used when harbor:namespace is unset.
	harborNamespaceDefault = "harbor"

	// harborServiceName is both the Helm release name and the resulting
	// Service name — this chart names the Service after the release, so the
	// two are always the same value here, not a general Kubernetes rule.
	harborServiceName = "harbor"

	harborPullSecretName           = "harbor-pull-secret"
	harborServicePatchResourceName = "harbor-service-https-passthrough"

	// harborRobotAccountName is the bare name used to create the robot
	// account. harborRobotUser is the login username Harbor derives from
	// it — Harbor always prefixes robot account names with "robot$" for the
	// actual credential — so the two are computed from one source instead
	// of two constants that could drift apart.
	harborRobotAccountName = "k8s-harbor-sa"
	harborRobotUser        = "robot$" + harborRobotAccountName

	nodeTrustNamespace = "node-registry-config"

	mirrorNamespace           = "harbor-image-mirror"
	mirrorImagesConfigMapName = "harbor-mirror-images"
	mirrorCredsSecretName     = "harbor-mirror-creds"
	mirrorPublicJobName       = "harbor-image-mirror-public"
	mirrorPinnedJobName       = "harbor-image-mirror-pinned"
	mirrorCronJobName         = "harbor-image-mirror"
)
