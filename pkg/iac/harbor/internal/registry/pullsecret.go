package registry

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const pullSecretName = "harbor-pull-secret"

func NewPullSecret(
	ctx *pulumi.Context,
	provider *kubernetes.Provider,
	namespace *corev1.Namespace,
	namespaceName string,
	registryHostname string,
	username string,
	password pulumi.StringOutput,
	release *helmv3.Release,
) (*corev1.Secret, error) {
	dockerConfigJSON := password.ApplyT(
		func(password string) (string, error) {
			auth := base64.StdEncoding.EncodeToString(
				[]byte(username + ":" + password),
			)

			dockerConfig := map[string]any{
				"auths": map[string]any{
					registryHostname: map[string]any{
						"auth": auth,
					},
				},
			}

			data, err := json.Marshal(dockerConfig)
			if err != nil {
				return "", fmt.Errorf(
					"marshal docker config: %w",
					err,
				)
			}

			return base64.StdEncoding.EncodeToString(
				data,
			), nil
		},
	).(pulumi.StringOutput)

	return corev1.NewSecret(
		ctx,
		pullSecretName,
		&corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(
					pullSecretName,
				),
				Namespace: pulumi.String(
					namespaceName,
				),
			},

			Type: pulumi.String(
				"kubernetes.io/dockerconfigjson",
			),

			Data: pulumi.StringMap{
				".dockerconfigjson": dockerConfigJSON,
			},
		},
		pulumi.Provider(provider),
		pulumi.DependsOn([]pulumi.Resource{
			namespace,
			release,
		}),
	)
}
