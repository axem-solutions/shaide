// knowledge-center: document storage, ingestion and retrieval for the
// optional Knowledge Center. Deployment (no persistent storage of its own)
// and ClusterIP Service on 8080. Reachable only from inside the cluster.
//
// KC keeps documents in the shared RustFS and document vectors in their own
// collections ("collection-<id>") of the shared Qdrant; it adds no storage
// workload or volume. It requests embeddings from shaide-server as an
// ordinary user account, never from a model provider directly.
//
// Callers:
//
//	shaide-server  /internal/v1/*      KNOWLEDGE_CENTER_INTERNAL_TOKEN
//	KC-MCP         /internal/mcp/v1/*  scoped token plus a signed context
//
// Probes reach GET /v1/health, which answers once the HTTP listener is up. It
// does not check RustFS, Qdrant or the embedding account: a Ready KC can
// still fail an upload until those are reachable and the account exists.
package knowledgecenter

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/platform"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	name      = knowledgecenter.ServiceName
	component = "knowledge-center"

	// The shaide-server Service, as KC requests embeddings from it.
	shaideServerURL = "http://shaide-server"
)

// Deploy creates the KC Deployment and its ClusterIP Service. The caller
// skips it when the Knowledge Center is not installed.
func Deploy(ctx *pulumi.Context, deps *runtime.DeploymentContext, cfg appconfig.Values, opts ...pulumi.ResourceOption) error {
	labels := deps.MetaLabels(name, component)

	resourceOpts := append([]pulumi.ResourceOption{deps.ProviderOpt, deps.NsOpt, deps.ConfigDeps}, opts...)
	_, err := appsv1.NewDeployment(ctx, name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    labels,
			Annotations: pulumi.StringMap{
				"pulumi.com/patchForce": pulumi.String("true"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			// One replica, replaced rather than rolled: KC serializes
			// lifecycle operations in-process.
			Replicas: pulumi.Int(1),
			Strategy: &appsv1.DeploymentStrategyArgs{
				Type: pulumi.String("Recreate"),
			},
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: deps.Labels(name),
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: labels,
				},
				Spec: podSpec(deps, cfg),
			},
		},
	}, resourceOpts...)
	if err != nil {
		return err
	}

	_, err = corev1.NewService(ctx, name+"-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    labels,
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("ClusterIP"),
			Selector: deps.Labels(name),
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(knowledgecenter.Port),
					TargetPort: pulumi.Int(knowledgecenter.Port),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, deps.ProviderOpt, deps.NsOpt)
	return err
}

func podSpec(deps *runtime.DeploymentContext, cfg appconfig.Values) *corev1.PodSpecArgs {
	uid := pulumi.Int(knowledgecenter.RuntimeUID)
	resources := cfg.KnowledgeCenter.Resources

	return &corev1.PodSpecArgs{
		// KC makes no Kubernetes API calls.
		AutomountServiceAccountToken: pulumi.Bool(false),
		Affinity: &corev1.AffinityArgs{
			NodeAffinity:    cfg.NodeAffinityFor(cfg.NodeSelectorKnowledgeCenter),
			PodAntiAffinity: deps.PodAntiAffinityFor(name),
		},
		// The image runs as 10001:10001. fsGroup makes the 0440 secret
		// files group-readable by it.
		SecurityContext: &corev1.PodSecurityContextArgs{
			RunAsUser:    uid,
			RunAsGroup:   uid,
			RunAsNonRoot: pulumi.Bool(true),
			FsGroup:      uid,
			SeccompProfile: &corev1.SeccompProfileArgs{
				Type: pulumi.String("RuntimeDefault"),
			},
		},
		Containers: corev1.ContainerArray{
			&corev1.ContainerArgs{
				Name:            pulumi.String(name),
				Image:           pulumi.String(cfg.Images.KnowledgeCenter),
				ImagePullPolicy: pulumi.String("Always"),
				Ports: corev1.ContainerPortArray{
					&corev1.ContainerPortArgs{
						ContainerPort: pulumi.Int(knowledgecenter.Port),
						Name:          pulumi.String("http"),
					},
				},
				Env: env(cfg),
				SecurityContext: &corev1.SecurityContextArgs{
					AllowPrivilegeEscalation: pulumi.Bool(false),
					ReadOnlyRootFilesystem:   pulumi.Bool(true),
					Capabilities: &corev1.CapabilitiesArgs{
						Drop: pulumi.StringArray{pulumi.String("ALL")},
					},
				},
				// Startup allows two minutes for the listener; readiness
				// and liveness then follow the HTTP health route only.
				StartupProbe:   healthProbe(5, 3, 24),
				ReadinessProbe: healthProbe(10, 3, 3),
				LivenessProbe:  healthProbe(20, 5, 3),
				Resources: &corev1.ResourceRequirementsArgs{
					Requests: pulumi.StringMap{
						"cpu":    pulumi.String(resources.CPURequest),
						"memory": pulumi.String(resources.MemoryRequest),
					},
					Limits: pulumi.StringMap{
						"cpu":    pulumi.String(resources.CPULimit),
						"memory": pulumi.String(resources.MemoryLimit),
					},
				},
				VolumeMounts: corev1.VolumeMountArray{
					&corev1.VolumeMountArgs{
						Name:      pulumi.String("kc-context"),
						MountPath: pulumi.String(knowledgecenter.SecretMountPath),
						ReadOnly:  pulumi.Bool(true),
					},
					&corev1.VolumeMountArgs{
						Name:      pulumi.String("tmp"),
						MountPath: pulumi.String("/tmp"),
					},
				},
			},
		},
		Volumes: corev1.VolumeArray{
			&corev1.VolumeArgs{
				Name: pulumi.String("kc-context"),
				Secret: &corev1.SecretVolumeSourceArgs{
					SecretName:  pulumi.String(platform.KCContextSecretName),
					DefaultMode: pulumi.Int(0o440),
				},
			},
			// Extraction and the OCR cache write here; the root filesystem
			// is read-only.
			&corev1.VolumeArgs{
				Name: pulumi.String("tmp"),
				EmptyDir: &corev1.EmptyDirVolumeSourceArgs{
					SizeLimit: pulumi.String("1Gi"),
				},
			},
		},
	}
}

func healthProbe(period, timeout, failureThreshold int) *corev1.ProbeArgs {
	return &corev1.ProbeArgs{
		HttpGet: &corev1.HTTPGetActionArgs{
			Path: pulumi.String(knowledgecenter.HealthPath),
			Port: pulumi.Int(knowledgecenter.Port),
		},
		PeriodSeconds:    pulumi.Int(period),
		TimeoutSeconds:   pulumi.Int(timeout),
		FailureThreshold: pulumi.Int(failureThreshold),
	}
}

// env is KC's runtime configuration. Credentials come from Secrets: the
// internal token and embedding password from knowledge-center, the RustFS
// credentials from the storage-owned Secret, and the context key and scoped
// token as files.
func env(cfg appconfig.Values) corev1.EnvVarArray {
	plain := []struct{ name, value string }{
		{"HOST", "0.0.0.0"},
		{"PORT", "8080"},
		{"S3_FQDN", cfg.ShaideEnv.S3FQDN},
		{"S3_PORT", cfg.ShaideEnv.S3Port},
		{"RUSTFS_WEBHOOK_ARN", cfg.ShaideEnv.RustFSWebhookARN},
		{"VECTOR_DB_URL", cfg.ShaideEnv.VectorDBUrl},
		{"SHAIDE_SERVER_URL", shaideServerURL},
		{"KC_SHAIDE_USERNAME", cfg.KnowledgeCenter.EmbeddingUsername},
		{"KC_CONTEXT_ISSUER", knowledgecenter.Issuer},
		{"KC_CONTEXT_TARGET", knowledgecenter.ServerID},
		{"KC_CONTEXT_AUDIENCE", knowledgecenter.KCAudience},
		{"KC_CONTEXT_MCP_AUDIENCE", knowledgecenter.MCPAudience},
		{"KC_CONTEXT_KID", knowledgecenter.KeyID},
		{"KC_CONTEXT_PUBLIC_KEY_FILE", knowledgecenter.PublicKeyFilePath},
		{"KC_MCP_SCOPED_TOKEN_FILE", knowledgecenter.ScopedTokenFilePath},
		{"TMPDIR", "/tmp"},
		{"XBERG_CACHE_DIR", "/tmp/xberg"},
	}

	vars := make(corev1.EnvVarArray, 0, len(plain)+4)
	for _, item := range plain {
		vars = append(vars, &corev1.EnvVarArgs{
			Name:  pulumi.String(item.name),
			Value: pulumi.String(item.value),
		})
	}

	secretRef := func(envName, secret, key string) *corev1.EnvVarArgs {
		return &corev1.EnvVarArgs{
			Name: pulumi.String(envName),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: pulumi.String(secret),
					Key:  pulumi.String(key),
				},
			},
		}
	}

	return append(vars,
		secretRef("KNOWLEDGE_CENTER_INTERNAL_TOKEN", platform.KCEnvSecretName, platform.KCEnvInternalToken),
		secretRef("KC_SHAIDE_PASSWORD", platform.KCEnvSecretName, platform.KCEnvEmbeddingPassword),
		secretRef("S3_USER", platform.StorageSecretName, platform.StorageAccessKey),
		secretRef("S3_PASSWORD", platform.StorageSecretName, platform.StorageSecretKey),
	)
}
