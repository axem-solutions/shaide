package harbor

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// SyncMode selects how private GHCR images are chosen for mirroring — see
// harbor:ghcrSyncMode in the stack config reference.
type SyncMode string

const (
	SyncModeAll        SyncMode = "all"
	SyncModeMinVersion SyncMode = "min-version"
	SyncModePinned     SyncMode = "pinned"
)

// harborConfig is every input DeployHarbor and the functions it calls need,
// read from Pulumi stack config exactly once. Passing this around instead of
// *config.Config means a function's signature shows exactly what it depends
// on, and there's one place (loadHarborConfig) to check for the full list of
// config keys this program reads.
type harborConfig struct {
	KubeconfigPath string
	KubeContext    string

	AdminPassword pulumi.StringOutput
	Namespace     string
	ChartPath     string

	// Platform is the cluster type the installer detected from the nodes'
	// spec.providerID ("gcp", "aws", "azure", "on-prem"). Storage placement
	// differs by platform: on-prem pins Harbor to NodeHostname and uses
	// hostPath, while cloud clusters rely on a dynamic StorageClass.
	Platform string

	NodeHostname    string
	StaticClusterIP string
	Projects        []string

	// RobotPasswordSet gates the pull secret, Harbor setup, and image mirror
	// altogether — see DeployHarbor. RobotPassword is only valid when this
	// is true.
	RobotPasswordSet bool
	RobotPassword    pulumi.StringOutput

	MirrorEnabled    bool
	PublicImages     string
	GhcrOrg          string
	GhcrUser         string
	GhcrToken        pulumi.StringOutput
	GhcrSyncMode     SyncMode
	GhcrMinVersions  string
	GhcrPinnedImages string
}

// loadHarborConfig reads every Pulumi stack config value this package uses,
// once. dir anchors a relative chartPath — see resolveChartPath.
func loadHarborConfig(ctx *pulumi.Context, dir string) harborConfig {
	root := config.New(ctx, "")
	conf := config.New(ctx, "harbor")

	chartPath := conf.Get("chartPath")
	if chartPath == "" {
		chartPath = defaultChartPath
	}

	namespace := conf.Get("namespace")
	if namespace == "" {
		namespace = harborNamespaceDefault
	}

	syncMode := SyncMode(conf.Get("ghcrSyncMode"))
	if syncMode == "" {
		syncMode = SyncModeAll
	}

	cfg := harborConfig{
		KubeconfigPath: root.Get("kubeconfig"),
		KubeContext:    root.Get("context"),

		// adminPassword has no optional path — a Harbor deploy without an
		// admin password isn't a valid deploy, so this fails fast (via
		// RequireSecret's panic, caught by the Pulumi engine) rather than
		// deferring to a confusing failure later.
		AdminPassword:   conf.RequireSecret("adminPassword"),
		Namespace:       namespace,
		ChartPath:       resolveChartPath(dir, chartPath),
		Platform:        conf.Get("platform"),
		NodeHostname:    conf.Get("nodeHostname"),
		StaticClusterIP: conf.Get("staticClusterIP"),
		Projects:        parseProjectNames(conf.Get("projects")),

		MirrorEnabled:    conf.GetBool("mirrorEnabled"),
		PublicImages:     conf.Get("publicImages"),
		GhcrOrg:          conf.Get("ghcrOrg"),
		GhcrUser:         conf.Get("ghcrUser"),
		GhcrToken:        conf.GetSecret("ghcrToken"),
		GhcrSyncMode:     syncMode,
		GhcrMinVersions:  conf.Get("ghcrMinVersions"),
		GhcrPinnedImages: conf.Get("ghcrPinnedImages"),
	}

	// Pull secret + Harbor setup + image mirror all require robotPassword —
	// on first pulumi up this key may be absent; set it then re-run pulumi
	// up. RequireSecret is only called once we know the key is actually
	// set, since RequireSecret itself panics on a missing key.
	if conf.Get("robotPassword") != "" {
		cfg.RobotPasswordSet = true
		cfg.RobotPassword = conf.RequireSecret("robotPassword")
	}

	return cfg
}
