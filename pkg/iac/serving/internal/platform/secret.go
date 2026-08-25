package platform

import (
	"encoding/json"
	"fmt"
	"strings"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	core_v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	meta_v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// registryHost extracts the registry hostname from an OCI reference.
// "harbor.harbor.svc.cluster.local/ai-models/nomic:1.5.0" → "harbor.harbor.svc.cluster.local"
func registryHost(ref string) string {
	if i := strings.IndexByte(ref, '/'); i > 0 {
		return ref[:i]
	}
	return ref
}

// CreateHarborPullSecret creates a kubernetes.io/dockerconfigjson pull secret that
// authenticates against the internal Harbor registry using the robot account credentials.
// The secret is named "harbor-creds" and must be referenced by chart imagePullSecrets.
func CreateHarborPullSecret(ctx *pulumi.Context, config *appConfig.Config, model appConfig.Model, llmdNamespace *core_v1.Namespace, opts ...pulumi.ResourceOption) (*core_v1.Secret, error) {
	dockerConfigJSON := config.HarborToken.ApplyT(func(token string) (string, error) {
		creds := map[string]string{
			"username": config.HarborUser,
			"password": token,
		}
		auths := map[string]any{
			config.HarborHostname: creds,
		}
		// On on-prem, harborHostname is the node-level DNS name used by containerd
		// (e.g. harbor.internal.lan), while harborRef uses the K8s service DNS name
		// (harbor.harbor.svc.cluster.local) so that ORAS can reach Harbor from inside
		// a pod. If those two hostnames differ, add credentials for the ORAS hostname
		// too — otherwise the ORAS pull Job will get a 401 Unauthorized.
		if model.ModelSource != nil {
			orasHost := registryHost(model.ModelSource.HarborRef)
			if orasHost != "" && orasHost != config.HarborHostname {
				auths[orasHost] = creds
			}
		}
		b, err := json.Marshal(map[string]any{"auths": auths})
		if err != nil {
			return "", err
		}
		return string(b), nil
	}).(pulumi.StringOutput)

	resourceName := fmt.Sprintf("harbor-creds-%s", model.Slug)
	baseOpts := []pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{llmdNamespace})}
	return core_v1.NewSecret(ctx, resourceName, &core_v1.SecretArgs{
		Metadata: &meta_v1.ObjectMetaArgs{
			Name:      pulumi.String("harbor-creds"),
			Namespace: pulumi.String(model.Namespace),
		},
		Type: pulumi.String("kubernetes.io/dockerconfigjson"),
		StringData: pulumi.StringMap{
			".dockerconfigjson": dockerConfigJSON,
		},
	}, append(baseOpts, opts...)...)
}
