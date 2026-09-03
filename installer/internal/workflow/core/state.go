package core

import (
	"github.com/axem-solutions/ai_platform/installer/internal/config"
	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	harborapi "github.com/axem-solutions/ai_platform/installer/internal/harbor/api"
	"github.com/axem-solutions/ai_platform/installer/internal/harbor/auth"
	"github.com/axem-solutions/ai_platform/pkg/kube"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type GlobalState struct {
	Discovery DiscoveryState
	Cluster   ClusterState
	Bootstrap BootstrapState
	Artifact  ArtifactState
	Pulumi    PulumiState
}

type ClusterState struct {
	SelectedContext string
	Client          kubernetes.Interface
	RESTConfig      *rest.Config
	ConfigPath      string
}

type DiscoveryState struct {
	Mode          Installation
	Target        *kube.ForwardTarget
	Client        *harborapi.Client
	Auth          auth.Credentials
	HarborForward *kube.Forward

	RobotPassword string
	AdminPassword string
}

type ArtifactState struct {
	SelectedModels []catalog.Model
	ModelOptions   []ModelOption
}
type ModelOption struct {
	Label string
	Model catalog.Model
}

type BootstrapState struct {
	Config  config.Config
	Catalog catalog.Catalog
	// GatewayHostname is populated from the gateway-provider template config
	// and reused downstream by stages that need to point HTTPRoutes at the
	// shared gateway (app-shaide).
	GatewayHostname string
	// CloudPlatform is detected from the cluster and can be confirmed or
	// overridden through the gateway-provider template. Downstream stages use
	// it to derive their own cloudProvider config without prompting again.
	CloudPlatform string
}

type PulumiState struct {
	ShaideAdmiPassword string
}

type ActiveStageState struct {
	Name string
	Data any
}

func (s *ActiveStageState) Begin(name string, data any, stepCount int) {
	s.Name = name
	s.Data = data
}

func (s *ActiveStageState) Reset() {
	s.Name = ""
	s.Data = nil
}

func NewGlobalState() *GlobalState {
	return &GlobalState{}
}

func NewActiveStageState() *ActiveStageState {
	return &ActiveStageState{}
}
