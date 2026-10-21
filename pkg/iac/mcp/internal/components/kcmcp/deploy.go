// KC-MCP: the Knowledge Center's MCP server, deployed with the Knowledge
// Center.
//
// Resources in the MCP namespace (app-mcp):
//   - Secret        kc-mcp-context  public.pem, gateway_token, scoped_token
//   - Deployment    kc-mcp
//   - Service       kc-mcp          ClusterIP, kc-mcp.app-mcp.svc.cluster.local:8080
//   - NetworkPolicy kc-mcp
//
// Signed retrieval runs agent -> shaide-server MCP gateway -> KC-MCP -> KC.
// shaide-server discovers KC-MCP through the same pod annotations as any MCP
// server (no static MCP_SERVERS), and forwards only when the discovered URL
// equals the origin of its signed target. KC-MCP verifies the gateway token
// and the signed context with the public key, then calls KC's scoped search
// with the scoped token. It holds no signing key, no KC internal token and no
// storage credentials.
//
// KC-MCP has no HTTP health route, so its probes check that the listener
// accepts TCP connections. Ready does not mean KC is reachable.
package kcmcp

import (
	"sort"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/config"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	name       = knowledgecenter.MCPServiceName
	secretName = "kc-mcp-context"

	nameLabel      = "app.kubernetes.io/name"
	namespaceLabel = "kubernetes.io/metadata.name"
)

// Deploy creates the KC-MCP resources. The caller skips it when the
// Knowledge Center is not installed.
func Deploy(ctx *pulumi.Context, cfg appconfig.Values, opts ...pulumi.ResourceOption) error {
	labels := pulumi.StringMap{
		nameLabel:                     pulumi.String(name),
		"app.kubernetes.io/component": pulumi.String("mcp-server"),
		"app.kubernetes.io/part-of":   pulumi.String("shaide"),
	}

	secret, err := corev1.NewSecret(ctx, secretName, &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(secretName),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    labels,
		},
		StringData: pulumi.StringMap{
			knowledgecenter.PublicKeyFile:    cfg.Secrets.KCContextPublicKey,
			knowledgecenter.GatewayTokenFile: cfg.Secrets.KCGatewayToken,
			knowledgecenter.ScopedTokenFile:  cfg.Secrets.KCScopedToken,
		},
	}, opts...)
	if err != nil {
		return err
	}

	deploymentOpts := append(append([]pulumi.ResourceOption{}, opts...), pulumi.DependsOn([]pulumi.Resource{secret}))
	deployment, err := appsv1.NewDeployment(ctx, name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
			Labels:    labels,
			Annotations: pulumi.StringMap{
				"pulumi.com/patchForce": pulumi.String("true"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: labels,
					// shaide-server's discovery reads these; the id is the
					// signed target's server_id.
					Annotations: pulumi.StringMap{
						"mcp.shaide/datasource": pulumi.String(knowledgecenter.ServerID),
						"mcp.shaide/url":        pulumi.String(knowledgecenter.MCPURL()),
					},
				},
				Spec: podSpec(cfg),
			},
		},
	}, deploymentOpts...)
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
			Selector: labels,
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(knowledgecenter.MCPPort),
					TargetPort: pulumi.Int(knowledgecenter.MCPPort),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, append(append([]pulumi.ResourceOption{}, opts...), pulumi.DependsOn([]pulumi.Resource{deployment}))...)
	if err != nil {
		return err
	}

	return deployNetworkPolicy(ctx, cfg, opts...)
}

func podSpec(cfg appconfig.Values) *corev1.PodSpecArgs {
	uid := pulumi.Int(knowledgecenter.RuntimeUID)
	resources := cfg.KnowledgeCenterMCP.Resources

	var pullSecrets corev1.LocalObjectReferenceArray
	for _, secret := range cfg.ImagePullSecrets {
		if secret != "" {
			pullSecrets = append(pullSecrets, &corev1.LocalObjectReferenceArgs{Name: pulumi.String(secret)})
		}
	}

	return &corev1.PodSpecArgs{
		// KC-MCP makes no Kubernetes API calls.
		AutomountServiceAccountToken: pulumi.Bool(false),
		Affinity:                     affinity(cfg),
		ImagePullSecrets:             pullSecrets,
		// The image runs as 10001:10001; fsGroup makes the 0440 secret files
		// group-readable by it.
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
				Image:           pulumi.String(cfg.Images.KnowledgeCenterMCP),
				ImagePullPolicy: pulumi.String(cfg.ImagePullPolicy),
				Ports: corev1.ContainerPortArray{
					&corev1.ContainerPortArgs{
						ContainerPort: pulumi.Int(knowledgecenter.MCPPort),
						Name:          pulumi.String("http"),
					},
				},
				Env: env(),
				SecurityContext: &corev1.SecurityContextArgs{
					AllowPrivilegeEscalation: pulumi.Bool(false),
					ReadOnlyRootFilesystem:   pulumi.Bool(true),
					Capabilities: &corev1.CapabilitiesArgs{
						Drop: pulumi.StringArray{pulumi.String("ALL")},
					},
				},
				StartupProbe:   tcpProbe(5, 3, 12),
				ReadinessProbe: tcpProbe(10, 3, 3),
				LivenessProbe:  tcpProbe(20, 5, 3),
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
					SecretName:  pulumi.String(secretName),
					DefaultMode: pulumi.Int(0o440),
				},
			},
			&corev1.VolumeArgs{
				Name: pulumi.String("tmp"),
				EmptyDir: &corev1.EmptyDirVolumeSourceArgs{
					SizeLimit: pulumi.String("64Mi"),
				},
			},
		},
	}
}

func env() corev1.EnvVarArray {
	vars := []struct{ name, value string }{
		{"HOST", "0.0.0.0"},
		{"PORT", "8080"},
		{"KNOWLEDGE_CENTER_URL", knowledgecenter.URL()},
		{"KC_CONTEXT_ISSUER", knowledgecenter.Issuer},
		{"KC_CONTEXT_TARGET", knowledgecenter.ServerID},
		{"KC_CONTEXT_AUDIENCE", knowledgecenter.MCPAudience},
		{"KC_CONTEXT_KC_AUDIENCE", knowledgecenter.KCAudience},
		{"KC_CONTEXT_KID", knowledgecenter.KeyID},
		{"KC_CONTEXT_PUBLIC_KEY_FILE", knowledgecenter.PublicKeyFilePath},
		{"KC_MCP_GATEWAY_TOKEN_FILE", knowledgecenter.GatewayTokenFilePath},
		{"KC_MCP_SCOPED_TOKEN_FILE", knowledgecenter.ScopedTokenFilePath},
		// KC is plain HTTP inside the cluster, which KC-MCP accepts only
		// with this opt-in.
		{"KC_CONTEXT_ALLOW_LOCAL_HTTP", "true"},
		// shaide-server sends the Service host it forwards to.
		{"KC_MCP_ALLOWED_HOSTS", knowledgecenter.MCPHost()},
	}

	out := make(corev1.EnvVarArray, 0, len(vars))
	for _, v := range vars {
		out = append(out, &corev1.EnvVarArgs{Name: pulumi.String(v.name), Value: pulumi.String(v.value)})
	}
	return out
}

func tcpProbe(period, timeout, failureThreshold int) *corev1.ProbeArgs {
	return &corev1.ProbeArgs{
		TcpSocket: &corev1.TCPSocketActionArgs{
			Port: pulumi.Int(knowledgecenter.MCPPort),
		},
		PeriodSeconds:    pulumi.Int(period),
		TimeoutSeconds:   pulumi.Int(timeout),
		FailureThreshold: pulumi.Int(failureThreshold),
	}
}

// affinity prefers the installer's placement (the CPU pools), else the MCP
// node selector. Soft, so KC-MCP still schedules when those nodes are full.
func affinity(cfg appconfig.Values) *corev1.AffinityArgs {
	var terms corev1.PreferredSchedulingTermArray
	add := func(key string, values []string) {
		values = append([]string(nil), values...)
		sort.Strings(values)
		terms = append(terms, &corev1.PreferredSchedulingTermArgs{
			Weight: pulumi.Int(100),
			Preference: &corev1.NodeSelectorTermArgs{
				MatchExpressions: corev1.NodeSelectorRequirementArray{
					&corev1.NodeSelectorRequirementArgs{
						Key:      pulumi.String(key),
						Operator: pulumi.String("In"),
						Values:   pulumi.ToStringArray(values),
					},
				},
			},
		})
	}

	switch {
	case len(cfg.Placement) > 0:
		for _, term := range cfg.Placement {
			add(term.Key, term.Values)
		}
	case cfg.NodeSelectorKey != "" && cfg.NodeSelector != "":
		add(cfg.NodeSelectorKey, []string{cfg.NodeSelector})
	default:
		return nil
	}

	return &corev1.AffinityArgs{
		NodeAffinity: &corev1.NodeAffinityArgs{PreferredDuringSchedulingIgnoredDuringExecution: terms},
	}
}

// deployNetworkPolicy admits shaide-server and lets KC-MCP reach only KC and
// DNS. The namespace-wide allow-shaide-ingress policy also selects KC-MCP and
// admits every app-shaide pod; allow rules add up, so that broader rule still
// applies, and the gateway token remains the actual caller check.
func deployNetworkPolicy(ctx *pulumi.Context, cfg appconfig.Values, opts ...pulumi.ResourceOption) error {
	tcp := func(port int) networkingv1.NetworkPolicyPortArray {
		return networkingv1.NetworkPolicyPortArray{
			&networkingv1.NetworkPolicyPortArgs{Protocol: pulumi.String("TCP"), Port: pulumi.Int(port)},
		}
	}
	shaide := func(pod string) *networkingv1.NetworkPolicyPeerArgs {
		return &networkingv1.NetworkPolicyPeerArgs{
			NamespaceSelector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{namespaceLabel: pulumi.String(cfg.ShaideNamespace)}},
			PodSelector:       &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{nameLabel: pulumi.String(pod)}},
		}
	}

	_, err := networkingv1.NewNetworkPolicy(ctx, name+"-netpol", &networkingv1.NetworkPolicyArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(cfg.Namespace),
		},
		Spec: &networkingv1.NetworkPolicySpecArgs{
			PodSelector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{nameLabel: pulumi.String(name)}},
			PolicyTypes: pulumi.StringArray{pulumi.String("Ingress"), pulumi.String("Egress")},
			Ingress: networkingv1.NetworkPolicyIngressRuleArray{
				&networkingv1.NetworkPolicyIngressRuleArgs{
					From:  networkingv1.NetworkPolicyPeerArray{shaide("shaide-server")},
					Ports: tcp(knowledgecenter.MCPPort),
				},
			},
			Egress: networkingv1.NetworkPolicyEgressRuleArray{
				&networkingv1.NetworkPolicyEgressRuleArgs{
					To:    networkingv1.NetworkPolicyPeerArray{shaide(knowledgecenter.ServiceName)},
					Ports: tcp(knowledgecenter.Port),
				},
				// DNS to any destination: NodeLocal DNSCache listens outside
				// any namespace.
				&networkingv1.NetworkPolicyEgressRuleArgs{
					Ports: networkingv1.NetworkPolicyPortArray{
						&networkingv1.NetworkPolicyPortArgs{Protocol: pulumi.String("UDP"), Port: pulumi.Int(53)},
						&networkingv1.NetworkPolicyPortArgs{Protocol: pulumi.String("TCP"), Port: pulumi.Int(53)},
					},
				},
			},
		},
	}, opts...)
	return err
}
