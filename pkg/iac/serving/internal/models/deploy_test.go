package models

import (
	"sync"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/modeldef"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks struct {
	mu        sync.Mutex
	resources map[string]resource.PropertyMap
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources[args.TypeToken+"::"+args.Name] = args.Inputs
	return args.Name + "-id", args.Inputs, nil
}

func (m *mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func model(t *testing.T, definition string) config.Model {
	t.Helper()

	def, err := modeldef.Parse([]byte(definition))
	if err != nil {
		t.Fatal(err)
	}

	return config.Model{
		Name:         "Example",
		Enabled:      true,
		Namespace:    "llm-d-example",
		ReleaseName:  "infra-example",
		NodeSelector: map[string]string{"nodegroup": "generative"},
		ModelPaths:   config.ModelPaths{Slug: def.Slug},
		Definition:   def,
		ModelSource: &config.ModelSource{
			HarborRef:   "harbor.harbor.svc.cluster.local/ai-models/example:0123456789ab",
			ModelUri:    "hub/org/example",
			StorageSize: "10Gi",
		},
	}
}

// A model registers its whole chain: weights, Gateway, router, server and
// route, plus the embedding Service for embedders.
func TestDeployModelRegistersTheServingChain(t *testing.T) {
	definition := `
slug: example
model: {name: org/example, size: 10Gi}
server:
  replicas: 2
  runtimeMetadata: true
  resources:
    limits: {nvidia.com/gpu: "1", memory: 8Gi}
router:
  replicas: 2
`
	for _, category := range []string{config.CategoryGenerative, config.CategoryEmbedder} {
		t.Run(category, func(t *testing.T) {
			m := &mocks{resources: map[string]resource.PropertyMap{}}

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				cfg := config.Values{
					Platform:         cluster.GCP,
					RouterChartPath:  "/charts/llm-d-router-gateway",
					ModelServerImage: "harbor/services/vllm/vllm-openai:v0.31.0",
					RouterImage:      "harbor/services/llm-d/llm-d-router-endpoint-picker:v1.0.0",
					ORASImage:        config.DefaultORASImage,
					Toleration:       &config.DefaultGPUToleration,
				}
				cfg.Harbor.Hostname = "harbor.harbor.svc.cluster.local"
				cfg.Harbor.User = "robot$puller"
				cfg.Harbor.Token = pulumi.ToSecret(pulumi.String("token")).(pulumi.StringOutput)
				cfg.Harbor.TokenSet = true

				provider, err := kubernetes.NewProvider(ctx, "test", &kubernetes.ProviderArgs{})
				if err != nil {
					return err
				}

				return deployModel(ctx, provider, cfg, model(t, definition), category)
			}, pulumi.WithMocks("app-serving", "test", m))
			if err != nil {
				t.Fatalf("deployModel: %v", err)
			}

			want := []string{
				"kubernetes:core/v1:Namespace::llm-d-example",
				"kubernetes:core/v1:Secret::harbor-creds-example",
				"kubernetes:core/v1:PersistentVolumeClaim::example-model",
				"kubernetes:batch/v1:Job::example-model-pull",
				"kubernetes:core/v1:ConfigMap::gateway-example-parameters",
				"kubernetes:gateway.networking.k8s.io/v1:Gateway::gateway-example",
				"kubernetes:core/v1:Service::llmd-gateway-example-example",
				"kubernetes:helm.sh/v3:Release::router-example",
				"kubernetes:core/v1:ConfigMap::model-example-runtime-metadata",
				"kubernetes:yaml/v2:ConfigGroup::model-example",
				"kubernetes:gateway.networking.k8s.io/v1:HTTPRoute::llm-d-inference-scheduling-example",
			}
			if category == config.CategoryEmbedder {
				want = append(want, "kubernetes:core/v1:Service::embedding-svc-example")
			}
			for _, key := range want {
				if _, ok := m.resources[key]; !ok {
					t.Errorf("missing %s", key)
				}
			}

			release := m.resources["kubernetes:helm.sh/v3:Release::router-example"]
			if release["chart"].StringValue() != "/charts/llm-d-router-gateway" || release["name"].StringValue() != "router-example" {
				t.Errorf("router release = %v", release)
			}

			group := m.resources["kubernetes:yaml/v2:ConfigGroup::model-example"]
			objs := group["objs"].ArrayValue()
			if len(objs) != 1 {
				t.Fatalf("ConfigGroup objs = %v", group["objs"])
			}
			deployment := objs[0].ObjectValue()
			if deployment["kind"].StringValue() != "Deployment" {
				t.Errorf("object = %v, want the model server Deployment", deployment["kind"])
			}
			if replicas := deployment["spec"].ObjectValue()["replicas"].NumberValue(); replicas != 2 {
				t.Errorf("replicas = %v, want 2", replicas)
			}
		})
	}
}
