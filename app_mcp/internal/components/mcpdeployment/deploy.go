// MCP Server Deployment + Service.
//
// Creates per-datasource Kubernetes resources:
//   - ConfigMap  mcp-<name>-ca     (only when Datasource.CACert is set)
//   - Deployment mcp-<name>
//   - Service    mcp-<name>        ClusterIP, reachable at mcp-<name>.mcp-gateway.svc.cluster.local
//
// CA certificate resolution (first match wins):
//  1. ds.CACert != ""          → creates mcp-<name>-ca ConfigMap for this datasource only
//  2. cfg.CompanyCACert != ""  → mounts the company-internal-ca ConfigMap (created by caconfigmap.Deploy)
//  3. neither                  → no cert, no mount
//
// Pod labels applied to every deployment:
//
//	app.kubernetes.io/name:      mcp-<datasource>
//	app.kubernetes.io/component: mcp-server    ← used by Shaide's Kubernetes API Watch selector
//	app.kubernetes.io/part-of:   shaide
package mcpdeployment

import (
	"app_mcp/internal/components/caconfigmap"
	appconfig "app_mcp/internal/config"
	"fmt"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// caMountPath is where the CA certificate is placed inside the container.
	caMountPath = "/etc/ssl/certs/datasource-ca.crt"

	// defaultPort is the MCP Server listen port used when Datasource.Port is not set.
	defaultPort = 8080

	// defaultReplicas is the pod replica count used when neither the datasource nor the
	// global config specifies a value.
	defaultReplicas = 1
)

// resolveStr returns the datasource-level value when non-empty, otherwise the global fallback.
func resolveStr(dsVal, globalVal string) string {
	if dsVal != "" {
		return dsVal
	}
	return globalVal
}

// resolveInt returns the datasource-level value when non-zero, otherwise the global fallback.
func resolveInt(dsVal, globalVal int) int {
	if dsVal != 0 {
		return dsVal
	}
	return globalVal
}

// Deploy creates the ConfigMap (when CACert is set), Deployment, and Service for a
// single MCP Server datasource. opts are forwarded to all resources.
func Deploy(ctx *pulumi.Context, ds appconfig.Datasource, cfg appconfig.Config, opts ...pulumi.ResourceOption) error {
	name := "mcp-" + ds.Name

	port := ds.Port
	if port == 0 {
		port = defaultPort
	}

	labels := pulumi.StringMap{
		"app.kubernetes.io/name":      pulumi.String(name),
		"app.kubernetes.io/component": pulumi.String("mcp-server"),
		"app.kubernetes.io/part-of":   pulumi.String("shaide"),
	}

	// --- CA ConfigMap resolution ---
	// Per-datasource cert takes precedence over the shared cert.
	// The company-internal-ca ConfigMap is created by caconfigmap.Deploy in main.go
	// before this function is called; we only reference its name here.
	caConfigMapName := ""
	switch {
	case ds.CACert != "":
		caConfigMapName = name + "-ca"
		cm, err := corev1.NewConfigMap(ctx, caConfigMapName, &corev1.ConfigMapArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(caConfigMapName),
				Namespace: pulumi.String(cfg.Namespace),
			},
			Data: pulumi.StringMap{
				"ca.crt": pulumi.String(ds.CACert),
			},
		}, opts...)
		if err != nil {
			return err
		}
		opts = append(opts, pulumi.DependsOn([]pulumi.Resource{cm}))
	case cfg.CompanyCACert != "":
		caConfigMapName = caconfigmap.CompanyCAName
	}

	// Resolve the effective trust env var: per-datasource takes precedence, company-level is the fallback.
	caTrustEnvVar := resolveStr(ds.CATrustEnvVar, cfg.CompanyCATrustEnvVar)

	// A cert without a trust env var is a silent misconfiguration: the ConfigMap and volume would
	// be created but never mounted, so the application cannot find the cert and TLS will fail.
	if caConfigMapName != "" && caTrustEnvVar == "" {
		return fmt.Errorf("datasource %q: caCert is set but caTrustEnvVar is empty — cert will not be trusted by the runtime", ds.Name)
	}

	// Resolve per-datasource overrides against global defaults.
	replicas := resolveInt(ds.Replicas, defaultReplicas)
	imagePullPolicy := resolveStr(ds.ImagePullPolicy, cfg.ImagePullPolicy)
	healthPath := resolveStr(ds.HealthPath, cfg.HealthPath)
	cpuRequest := resolveStr(ds.CPURequest, cfg.CPURequest)
	memoryRequest := resolveStr(ds.MemoryRequest, cfg.MemoryRequest)
	cpuLimit := resolveStr(ds.CPULimit, cfg.CPULimit)
	memoryLimit := resolveStr(ds.MemoryLimit, cfg.MemoryLimit)
	startupProbeFailureThreshold := resolveInt(ds.StartupProbeFailureThreshold, cfg.StartupProbeFailureThreshold)

	mainContainer := buildMainContainer(name, ds.Image, caTrustEnvVar, caConfigMapName, port, imagePullPolicy, healthPath, cpuRequest, memoryRequest, cpuLimit, memoryLimit, startupProbeFailureThreshold, ds.DisableProbes, ds.Args, ds.Env, ds.SecretEnv, cfg)
	volumes := buildVolumes(caConfigMapName)
	nodeAffinity := buildNodeAffinity(cfg.NodeSelectorKey, resolveStr(ds.NodeSelector, cfg.NodeSelector))
	imagePullSecrets := buildImagePullSecrets(cfg.ImagePullSecrets)

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
			Replicas: pulumi.Int(replicas),
			Strategy: &appsv1.DeploymentStrategyArgs{
				Type:          pulumi.String("Recreate"),
				RollingUpdate: nil,
			},
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: labels,
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: labels,
					Annotations: pulumi.StringMap{
						"mcp.shaide/datasource": pulumi.String(name),
						"mcp.shaide/url":        pulumi.String(fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", name, cfg.Namespace, port)),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Affinity:         nodeAffinity,
					ImagePullSecrets: imagePullSecrets,
					Containers:       corev1.ContainerArray{mainContainer},
					Volumes:          volumes,
				},
			},
		},
	}, opts...)
	if err != nil {
		return err
	}

	svcOpts := append(opts, pulumi.DependsOn([]pulumi.Resource{deployment}))
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
					Port:       pulumi.Int(port),
					TargetPort: pulumi.Int(port),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, svcOpts...)
	return err
}

func buildNodeAffinity(key, value string) *corev1.AffinityArgs {
	if key == "" || value == "" {
		return nil
	}
	return &corev1.AffinityArgs{
		NodeAffinity: &corev1.NodeAffinityArgs{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelectorArgs{
				NodeSelectorTerms: corev1.NodeSelectorTermArray{
					&corev1.NodeSelectorTermArgs{
						MatchExpressions: corev1.NodeSelectorRequirementArray{
							&corev1.NodeSelectorRequirementArgs{
								Key:      pulumi.String(key),
								Operator: pulumi.String("In"),
								Values:   pulumi.StringArray{pulumi.String(value)},
							},
						},
					},
				},
			},
		},
	}
}

func buildImagePullSecrets(names []string) corev1.LocalObjectReferenceArray {
	if len(names) == 0 {
		return nil
	}
	refs := corev1.LocalObjectReferenceArray{}
	for _, name := range names {
		if name == "" {
			continue
		}
		refs = append(refs, &corev1.LocalObjectReferenceArgs{
			Name: pulumi.String(name),
		})
	}
	return refs
}

// buildVolumes returns the pod volume list. The CA volume is included only when
// caConfigMapName is non-empty (i.e. the datasource has a CACert configured).
func buildVolumes(caConfigMapName string) corev1.VolumeArray {
	if caConfigMapName == "" {
		return nil
	}
	return corev1.VolumeArray{
		&corev1.VolumeArgs{
			Name: pulumi.String("datasource-ca"),
			ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
				Name: pulumi.String(caConfigMapName),
			},
		},
	}
}

// buildMainContainer returns the MCP Server main container spec.
// caConfigMapName non-empty means a cert is available: mount it and set the trust env var.
// caTrustEnvVar is the runtime-specific env var (NODE_EXTRA_CA_CERTS / REQUESTS_CA_BUNDLE /
// SSL_CERT_FILE). If caConfigMapName is empty, no volume mount or trust env var is added.
// imagePullPolicy, healthPath, and resource values are already resolved against global defaults
// by the caller.
func buildMainContainer(name, image, caTrustEnvVar, caConfigMapName string, port int, imagePullPolicy, healthPath, cpuRequest, memoryRequest, cpuLimit, memoryLimit string, startupProbeFailureThreshold int, disableProbes bool, args []string, env map[string]string, secretEnv []appconfig.SecretEnvVar, cfg appconfig.Config) *corev1.ContainerArgs {
	var envVars corev1.EnvVarArray
	var volumeMounts corev1.VolumeMountArray

	if caConfigMapName != "" && caTrustEnvVar != "" {
		envVars = append(envVars, &corev1.EnvVarArgs{
			Name:  pulumi.String(caTrustEnvVar),
			Value: pulumi.String(caMountPath),
		})
		volumeMounts = corev1.VolumeMountArray{
			&corev1.VolumeMountArgs{
				Name:      pulumi.String("datasource-ca"),
				MountPath: pulumi.String(caMountPath),
				SubPath:   pulumi.String("ca.crt"),
				ReadOnly:  pulumi.Bool(true),
			},
		}
	}

	for k, v := range env {
		envVars = append(envVars, &corev1.EnvVarArgs{
			Name:  pulumi.String(k),
			Value: pulumi.String(v),
		})
	}
	for _, ref := range secretEnv {
		if ref.Name == "" || ref.SecretName == "" || ref.SecretKey == "" {
			continue
		}
		envVars = append(envVars, &corev1.EnvVarArgs{
			Name: pulumi.String(ref.Name),
			ValueFrom: &corev1.EnvVarSourceArgs{
				SecretKeyRef: &corev1.SecretKeySelectorArgs{
					Name: pulumi.String(ref.SecretName),
					Key:  pulumi.String(ref.SecretKey),
				},
			},
		})
	}

	var containerArgs pulumi.StringArray
	for _, arg := range args {
		containerArgs = append(containerArgs, pulumi.String(arg))
	}

	healthProbe := func(initialDelay, period, timeout, failureThreshold int) *corev1.ProbeArgs {
		p := &corev1.ProbeArgs{
			HttpGet: &corev1.HTTPGetActionArgs{
				Path: pulumi.String(healthPath),
				Port: pulumi.Int(port),
			},
			PeriodSeconds:    pulumi.Int(period),
			TimeoutSeconds:   pulumi.Int(timeout),
			FailureThreshold: pulumi.Int(failureThreshold),
		}
		if initialDelay > 0 {
			p.InitialDelaySeconds = pulumi.Int(initialDelay)
		}
		return p
	}

	container := &corev1.ContainerArgs{
		Name:            pulumi.String(name),
		Image:           pulumi.String(image),
		ImagePullPolicy: pulumi.String(imagePullPolicy),
		Args:            containerArgs,
		Ports: corev1.ContainerPortArray{
			&corev1.ContainerPortArgs{
				ContainerPort: pulumi.Int(port),
				Name:          pulumi.String("http"),
			},
		},
		Env:          envVars,
		VolumeMounts: volumeMounts,
		Resources: &corev1.ResourceRequirementsArgs{
			Requests: pulumi.StringMap{
				"cpu":    pulumi.String(cpuRequest),
				"memory": pulumi.String(memoryRequest),
			},
			Limits: pulumi.StringMap{
				"cpu":    pulumi.String(cpuLimit),
				"memory": pulumi.String(memoryLimit),
			},
		},
	}
	if !disableProbes {
		// StartupProbe gates liveness/readiness until the MCP Server is up.
		// failureThreshold * periodSeconds = the max startup window.
		container.StartupProbe = healthProbe(0, cfg.StartupProbePeriod, cfg.StartupProbeTimeout, startupProbeFailureThreshold)
		container.ReadinessProbe = healthProbe(cfg.ReadinessProbeInitialDelay, cfg.ReadinessProbePeriod, cfg.ReadinessProbeTimeout, cfg.ReadinessProbeFailureThreshold)
		container.LivenessProbe = healthProbe(cfg.LivenessProbeInitialDelay, cfg.LivenessProbePeriod, cfg.LivenessProbeTimeout, cfg.LivenessProbeFailureThreshold)
	}
	return container
}
