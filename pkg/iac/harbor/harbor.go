package harbor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	k8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// defaultChartPath is used when the stack config does not set harbor:chartPath.
// It is relative to the project directory, so it goes through resolveChartPath
// like any configured value.
const defaultChartPath = "./charts/harbor-1.18.2.tgz"

func buildHarborValues(cfg harborConfig) pulumi.Map {
	externalURL := fmt.Sprintf("http://harbor.%s.svc.cluster.local", cfg.Namespace)

	persistence := pulumi.Map{
		"enabled":        pulumi.Bool(true),
		"resourcePolicy": pulumi.String("keep"),
		"persistentVolumeClaim": pulumi.Map{
			"registry": pulumi.Map{
				"size":       pulumi.String("100Gi"),
				"accessMode": pulumi.String("ReadWriteOnce"),
			},
			"jobservice": pulumi.Map{
				"jobLog": pulumi.Map{
					"size":       pulumi.String("1Gi"),
					"accessMode": pulumi.String("ReadWriteOnce"),
				},
			},
			"database": pulumi.Map{
				"size":       pulumi.String("2Gi"),
				"accessMode": pulumi.String("ReadWriteOnce"),
			},
			"redis": pulumi.Map{
				"size":       pulumi.String("1Gi"),
				"accessMode": pulumi.String("ReadWriteOnce"),
			},
		},
	}

	clusterIP := pulumi.Map{
		"name": pulumi.String(harborServiceName),
		"ports": pulumi.Map{
			"httpPort":  pulumi.Int(80),
			"httpsPort": pulumi.Int(443),
		},
	}
	// Pinning the ClusterIP makes it known at Pulumi-program time, which is what
	// lets deployNodeTrust bake it into every node's hosts.toml without reading
	// the Service back out of the cluster after the Helm release creates it.
	if cfg.StaticClusterIP != "" {
		clusterIP["staticClusterIP"] = pulumi.String(cfg.StaticClusterIP)
	}

	values := pulumi.Map{
		"expose": pulumi.Map{
			"type":      pulumi.String("clusterIP"),
			"clusterIP": clusterIP,
			"tls": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
		},
		"externalURL":         pulumi.String(externalURL),
		"harborAdminPassword": cfg.AdminPassword,
		"updateStrategy":      pulumi.Map{"type": pulumi.String("Recreate")},
		"persistence":         persistence,
		"trivy":               pulumi.Map{"enabled": pulumi.Bool(false)},
	}

	// Pin all Harbor pods to a specific node when nodeHostname is provided.
	if cfg.NodeHostname != "" {
		nodeSelector := pulumi.StringMap{
			"kubernetes.io/hostname": pulumi.String(cfg.NodeHostname),
		}
		values["nodeSelector"] = nodeSelector
		for _, component := range []string{"core", "portal", "jobservice", "nginx"} {
			values[component] = pulumi.Map{"nodeSelector": nodeSelector}
		}
		values["registry"] = pulumi.Map{"nodeSelector": nodeSelector}
		values["database"] = pulumi.Map{"nodeSelector": nodeSelector}
		values["redis"] = pulumi.Map{"nodeSelector": nodeSelector}
	}

	return values
}

func createHarborPullSecret(
	ctx *pulumi.Context,
	robotSecret pulumi.StringOutput,
	k8sProvider *kubernetes.Provider,
	ns *corev1.Namespace,
	release *helmv3.Release,
	cfg harborConfig,
) error {
	harborHostname := fmt.Sprintf("harbor.%s.svc.cluster.local", cfg.Namespace)

	dockerConfigJSON := robotSecret.ApplyT(func(secret string) (string, error) {
		auth := base64.StdEncoding.EncodeToString([]byte(harborRobotUser + ":" + secret))
		dockerConfig := map[string]any{
			"auths": map[string]any{
				harborHostname: map[string]any{"auth": auth},
			},
		}
		jsonBytes, err := json.Marshal(dockerConfig)
		if err != nil {
			return "", fmt.Errorf("marshal docker config: %w", err)
		}
		return base64.StdEncoding.EncodeToString(jsonBytes), nil
	}).(pulumi.StringOutput)

	_, err := corev1.NewSecret(ctx, harborPullSecretName, &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(harborPullSecretName),
			Namespace: pulumi.String(cfg.Namespace),
		},
		Type: pulumi.String("kubernetes.io/dockerconfigjson"),
		Data: pulumi.StringMap{
			".dockerconfigjson": dockerConfigJSON,
		},
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn([]pulumi.Resource{ns, release}))

	return err
}

// fixHarborHTTPSPortBlackhole adds a 443 port to Harbor's Service, routed to
// the same plain-HTTP backend as port 80.
//
// Why this is needed: the harbor-helm chart only emits a Service port for
// HTTPS when tls.enabled=true — this deployment always runs with TLS
// disabled (see buildHarborValues), so without this fix the Service has
// *no* port 443 rule at all. Container registry clients (skopeo, docker,
// containerd) always attempt an HTTPS probe first, even with
// --tls-verify=false — that flag only skips certificate validation, it does
// not skip attempting TLS in the first place. Hitting a ClusterIP port with
// no matching Service rule doesn't get refused, it gets silently
// black-holed: the connection just hangs until the OS-level TCP timeout.
// Measured live against this exact cluster: ~30s per attempt, twice per
// `skopeo inspect`/`copy` call (once for the registry ping, once for the
// manifest fetch — each a separate HTTP request with no memoized "this host
// is HTTP-only" state) — i.e. ~60s of dead time added to every single image
// mirrored, before it even gets to the (fast) HTTP fallback that eventually
// succeeds.
//
// Routing 443 to the same backend doesn't add real TLS support — nginx
// still only speaks plain HTTP there. The point is purely that a TLS
// handshake attempted against a plain-HTTP server fails *fast* (the server
// responds, just not in the TLS protocol, so the client errors out almost
// immediately) instead of the current black hole. Fast failure → fast
// fallback to HTTP → the ~60s/image tax disappears.
//
// Implemented as a Service patch (server-side apply) rather than by editing
// buildHarborValues' clusterIP.ports.httpsPort, because that Helm value is
// exactly what's already set and exactly what the chart ignores while TLS
// stays disabled — editing it further would have no effect. A patch adds
// the port 443 entry under this program's own field ownership without
// taking over the rest of the Service, which stays Helm-owned.
func fixHarborHTTPSPortBlackhole(
	ctx *pulumi.Context,
	k8sProvider *kubernetes.Provider,
	namespace string,
	release *helmv3.Release,
) error {
	_, err := corev1.NewServicePatch(ctx, harborServicePatchResourceName, &corev1.ServicePatchArgs{
		Metadata: &metav1.ObjectMetaPatchArgs{
			Name:      pulumi.String(harborServiceName),
			Namespace: pulumi.String(namespace),
		},
		Spec: &corev1.ServiceSpecPatchArgs{
			Ports: corev1.ServicePortPatchArray{
				&corev1.ServicePortPatchArgs{
					Name: pulumi.String("https-passthrough"),
					Port: pulumi.Int(443),
					// Same target as the chart's own "http" port — see
					// harbor-1.18.2's nginx container port. Not derived
					// programmatically (the Helm Release resource is opaque
					// to Pulumi's resource graph), so if the chart version
					// ever changes its nginx containerPort, this needs a
					// matching update.
					TargetPort: pulumi.Int(8080),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, pulumi.Provider(k8sProvider), pulumi.DependsOn([]pulumi.Resource{release}))

	return err
}

// resolveChartPath anchors a relative chartPath to dir.
//
// A standalone `pulumi up` runs with the working directory set to the project
// directory, so a relative path from the stack file resolves correctly on its
// own. The installer instead runs this program through the automation API as an
// inline source: the program executes inside the installer process, whose
// working directory is unrelated to auto.WorkDir. Relative paths therefore have
// to be anchored explicitly against the project directory.
func resolveChartPath(dir, chartPath string) string {
	if dir == "" || chartPath == "" || filepath.IsAbs(chartPath) {
		return chartPath
	}

	return filepath.Join(dir, chartPath)
}

// DeployHarbor deploys Harbor from the bundled chart. dir is the project
// directory used to resolve a relative chartPath; pass "." when the program is
// run directly by the Pulumi CLI.
func DeployHarbor(ctx *pulumi.Context, dir string) error {
	cfg := loadHarborConfig(ctx, dir)

	// kubeconfig is optional.
	//   Cloud (GCP/AWS/Azure): omit — the provider reads KUBECONFIG env or
	//     the kubeconfig written by the cloud CLI (gcloud, aws, az).
	//   On-prem / CI with an explicit kubeconfig path: set kubeconfig in the
	//     stack config to target a specific cluster.
	providerArgs := &k8s.ProviderArgs{}

	if cfg.KubeconfigPath != "" {
		providerArgs.Kubeconfig = pulumi.StringPtr(cfg.KubeconfigPath)
	}

	if cfg.KubeContext != "" {
		providerArgs.Context = pulumi.StringPtr(cfg.KubeContext)
	}

	k8sProvider, err := k8s.NewProvider(ctx, "k8s", providerArgs)
	if err != nil {
		return err
	}

	providerOpt := pulumi.Provider(k8sProvider)

	// Namespace
	ns, err := corev1.NewNamespace(ctx, "harbor-namespace", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(cfg.Namespace),
		},
	}, providerOpt)
	if err != nil {
		return fmt.Errorf("harbor namespace: %w", err)
	}

	// Helm release
	release, err := helmv3.NewRelease(ctx, harborServiceName, &helmv3.ReleaseArgs{
		Chart:       pulumi.String(cfg.ChartPath),
		Namespace:   pulumi.String(cfg.Namespace),
		Name:        pulumi.String(harborServiceName),
		Values:      buildHarborValues(cfg),
		WaitForJobs: pulumi.Bool(true),
		Timeout:     pulumi.Int(600),
	}, providerOpt, pulumi.DependsOn([]pulumi.Resource{ns}))
	if err != nil {
		return fmt.Errorf("harbor helm release: %w", err)
	}

	if err := fixHarborHTTPSPortBlackhole(ctx, k8sProvider, cfg.Namespace, release); err != nil {
		return fmt.Errorf("harbor https passthrough port: %w", err)
	}

	// Pull secret + Harbor setup + image mirror all require harbor:robotPassword
	// to be set in stack config. On first pulumi up this key may be absent; set
	// it then re-run pulumi up.
	if cfg.RobotPasswordSet {
		// Declares the configured projects (public) and the robot account
		// in Harbor itself, via a real Pulumi provider — see setup.go. The
		// returned Output, not cfg.RobotPassword itself, is what's fed into
		// the pull secret and mirror below, so neither can be created
		// before the robot account actually exists in Harbor with this
		// password set.
		robotPassword, err := ensureHarborSetup(ctx, release, cfg)
		if err != nil {
			return fmt.Errorf("harbor setup: %w", err)
		}

		if err := createHarborPullSecret(ctx, robotPassword, k8sProvider, ns, release, cfg); err != nil {
			return fmt.Errorf("harbor pull secret: %w", err)
		}

		// Image mirroring is opt-in, separate from the pull secret above —
		// a cluster can pull from Harbor without this platform also being
		// responsible for keeping Harbor populated.
		if cfg.MirrorEnabled {
			if err := deployImageMirror(ctx, k8sProvider, cfg, robotPassword); err != nil {
				return fmt.Errorf("harbor image mirror: %w", err)
			}
		}
	}

	// Node-trust DaemonSet — requires a pinned ClusterIP (see buildHarborValues):
	// without one there is nothing stable to bake into every node's hosts.toml.
	if cfg.StaticClusterIP != "" {
		if err := deployNodeTrust(ctx, k8sProvider, cfg.StaticClusterIP, cfg.Namespace, release); err != nil {
			return fmt.Errorf("harbor node trust: %w", err)
		}
	}

	ctx.Export("harborReleaseName", release.Name)
	return nil
}
