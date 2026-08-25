package gpuoperator

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func createPullSecret(
	ctx *pulumi.Context,
	harborHostname string,
	robotSecret pulumi.StringOutput,
	provider pulumi.ProviderResource,
	dependsOn []pulumi.Resource,
) (*corev1.Secret, error) {
	const robotUser = "robot$k8s-harbor-sa"

	dockerConfigJSON := robotSecret.ApplyT(func(secret string) (string, error) {
		auth := base64.StdEncoding.EncodeToString([]byte(robotUser + ":" + secret))
		dockerConfig := map[string]interface{}{
			"auths": map[string]interface{}{
				harborHostname: map[string]interface{}{
					"auth": auth,
				},
			},
		}
		jsonBytes, err := json.Marshal(dockerConfig)
		if err != nil {
			return "", fmt.Errorf("marshalling docker config: %w", err)
		}
		return base64.StdEncoding.EncodeToString(jsonBytes), nil
	}).(pulumi.StringOutput)

	return corev1.NewSecret(ctx, "gpu-operator-harbor-pull-secret", &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("harbor-pull-secret"),
			Namespace: pulumi.String("gpu-operator"),
		},
		Type: pulumi.String("kubernetes.io/dockerconfigjson"),
		Data: pulumi.StringMap{
			".dockerconfigjson": dockerConfigJSON,
		},
	}, pulumi.Provider(provider), pulumi.DependsOn(dependsOn))
}
