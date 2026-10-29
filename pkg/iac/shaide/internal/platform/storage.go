package platform

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// The storage-owned Secret. RustFS takes its root credentials from it, and
// the Knowledge Center, which owns object storage operations, reads the same
// keys. shaide-server does not mount it.
//
// The values are the existing s3User and s3Password stack values, so moving
// them here neither rotates the credentials nor touches stored data.
const (
	StorageSecretName = "rustfs-credentials"

	StorageAccessKey = "access-key"
	StorageSecretKey = "secret-key"
	// StorageWebhookToken is the bearer token RustFS sends with object
	// notifications to shaide-server. Present only with the Knowledge Center.
	StorageWebhookToken = "notify-webhook-token"
)

// CreateStorageSecret creates the storage-owned Secret.
func CreateStorageSecret(ctx *pulumi.Context, cfg appconfig.Values, providerOpt, nsOpt pulumi.ResourceOption) (*corev1.Secret, error) {
	return corev1.NewSecret(ctx, StorageSecretName, &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(StorageSecretName),
			Namespace: pulumi.String(cfg.Namespace),
		},
		StringData: storageSecretData(cfg),
	}, providerOpt, nsOpt)
}

func storageSecretData(cfg appconfig.Values) pulumi.StringMap {
	data := pulumi.StringMap{
		StorageAccessKey: pulumi.String(cfg.ShaideEnv.S3User),
		StorageSecretKey: cfg.Secrets.S3Password,
	}
	if cfg.KnowledgeCenterEnabled {
		data[StorageWebhookToken] = cfg.Secrets.KCWebhookToken
	}

	return data
}
