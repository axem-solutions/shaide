package config

import (
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	stackconfig "github.com/axem-solutions/ai_platform/pkg/stack/config"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const Namespace = "harbor"

const (
	KeyPlatform             stackconfig.Key = "platform"
	KeyKubeconfig           stackconfig.Key = "kubeconfig"
	KeyContext              stackconfig.Key = "context"
	KeyAdminPassword        stackconfig.Key = "adminPassword"
	KeyRobotPassword        stackconfig.Key = "robotPassword"
	KeyNamespace            stackconfig.Key = "namespace"
	KeyChartPath            stackconfig.Key = "chartPath"
	KeyStorageMode          stackconfig.Key = "storageMode"
	KeyStorageClass         stackconfig.Key = "storageClass"
	KeyHostPathBase         stackconfig.Key = "hostPathBase"
	KeyNodeHostname         stackconfig.Key = "nodeHostname"
	KeyRegistryHostname     stackconfig.Key = "registryHostname"
	KeyStaticClusterIP      stackconfig.Key = "staticClusterIP"
	KeyNodeTrustEnabled     stackconfig.Key = "nodeTrustEnabled"
	KeyHTTPSFastFailEnabled stackconfig.Key = "httpsFastFailEnabled"
	KeyMirrorEnabled        stackconfig.Key = "mirrorEnabled"
	KeyPublicImages         stackconfig.Key = "publicImages"
	KeyGHCROrg              stackconfig.Key = "ghcrOrg"
	KeyGHCRUser             stackconfig.Key = "ghcrUser"
	KeyGHCRToken            stackconfig.Key = "ghcrToken"
	KeyGHCRSyncMode         stackconfig.Key = "ghcrSyncMode"
	KeyGHCRMinVersions      stackconfig.Key = "ghcrMinVersions"
	KeyGHCRPinnedImages     stackconfig.Key = "ghcrPinnedImages"
)

type Config struct {
	stack.Config
	definition stackconfig.Config[Values]
}

func New(projectDir string, opts stack.Options) Config {
	definition := newDefinition(opts)

	return Config{
		Config:     stack.NewConfig(Namespace, Namespace, projectDir, definition),
		definition: definition,
	}
}

func newDefinition(opts stack.Options) stackconfig.Config[Values] {
	return stackconfig.Config[Values]{
		Namespace: Namespace,
		Entries: []stackconfig.Entry[Values]{
			{
				Key: KeyPlatform,
				Source: stackconfig.Source{
					Value: string(opts.Platform),
				},
				Policy: stackconfig.Policy{
					Required: true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Platform = platform.Platform(
						root.Get(KeyPlatform.String()),
					)
				},
			},
			{
				Key: KeyKubeconfig,
				Source: stackconfig.Source{
					Value: opts.Kubeconfig,
				},
				Policy: stackconfig.Policy{
					Required: true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Kubernetes.KubeconfigPath = root.Get(KeyKubeconfig.String())
				},
			},
			{
				Key: KeyContext,
				Source: stackconfig.Source{
					Value: opts.Context,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Kubernetes.Context = root.Get(KeyContext.String())
				},
			},
			{
				Key: KeyAdminPassword,
				Prompt: &stackconfig.Prompt{
					Kind:  stackconfig.PromptInput,
					Title: "Harbor admin password",
				},
				Policy: stackconfig.Policy{
					Required: true,
					Secret:   true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Harbor.AdminPassword = root.RequireSecret(KeyAdminPassword.String())
				},
			},
			{
				Key: KeyRobotPassword,
				Prompt: &stackconfig.Prompt{
					Kind:  stackconfig.PromptInput,
					Title: "Harbor robot password",
				},
				Policy: stackconfig.Policy{
					Secret: true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					if root.Get(KeyRobotPassword.String()) == "" {
						return
					}

					cfg.Harbor.Robot.Configured = true
					cfg.Harbor.Robot.Password = root.RequireSecret(KeyRobotPassword.String())
				},
			},
			{
				Key: KeyNamespace,
				Source: stackconfig.Source{
					Default: DefaultNamespace,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Harbor.Namespace = root.Get(KeyNamespace.String())
				},
			},
			{
				Key: KeyChartPath,
				Source: stackconfig.Source{
					Default: DefaultChartPath,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Harbor.ChartPath = root.Get(KeyChartPath.String())
				},
			},
			{
				Key: KeyStorageMode,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Storage.Mode = StorageMode(root.Get(KeyStorageMode.String()))
				},
			},
			{
				Key: KeyStorageClass,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Storage.StorageClass = root.Get(KeyStorageClass.String())
				},
			},
			{
				Key: KeyHostPathBase,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Storage.HostPathBase = root.Get(KeyHostPathBase.String())
				},
			},
			{
				Key: KeyNodeHostname,
				Prompt: &stackconfig.Prompt{
					Kind:  stackconfig.PromptInput,
					Title: "Harbor storage node hostname",
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Storage.NodeHostname = root.Get(KeyNodeHostname.String())
				},
			},
			{
				Key: KeyGHCRUser,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Mirror.GHCR.User = root.Get(KeyGHCRUser.String())
				},
			},
			{
				Key:    KeyGHCRToken,
				Policy: stackconfig.Policy{Secret: true},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Mirror.GHCR.Token = root.GetSecret(KeyGHCRToken.String())
				},
			},
			{
				Key: KeyRegistryHostname,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Network.RegistryHostname = root.Get(KeyRegistryHostname.String())
				},
			},
			{
				Key: KeyStaticClusterIP,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Network.StaticClusterIP = root.Get(KeyStaticClusterIP.String())
				},
			},
			{
				Key: KeyNodeTrustEnabled,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Network.NodeTrustEnabled = root.GetBool(KeyNodeTrustEnabled.String())
				},
			},
			{
				Key: KeyHTTPSFastFailEnabled,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Network.HTTPSFastFailEnabled = root.GetBool(KeyHTTPSFastFailEnabled.String())
				},
			},
			{
				Key: KeyMirrorEnabled,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Mirror.Enabled = root.GetBool(KeyMirrorEnabled.String())
				},
			},
			{
				Key: KeyPublicImages,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Mirror.PublicImages = root.Get(KeyPublicImages.String())
				},
			},
			{
				Key: KeyGHCROrg,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Mirror.GHCR.Org = root.Get(KeyGHCROrg.String())
				},
			},
			{
				Key: KeyGHCRSyncMode,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Mirror.GHCR.SyncMode = SyncMode(root.Get(KeyGHCRSyncMode.String()))
				},
			},
			{
				Key: KeyGHCRMinVersions,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Mirror.GHCR.MinVersions = root.Get(KeyGHCRMinVersions.String())
				},
			},
			{
				Key: KeyGHCRPinnedImages,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Mirror.GHCR.PinnedImages = root.Get(KeyGHCRPinnedImages.String())
				},
			},
		},
	}
}
