package chart

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/config"
)

func BuildValues(cfg config.Config) pulumi.Map {
	values := pulumi.Map{
		"expose": pulumi.Map{
			"type": pulumi.String("clusterIP"),

			"clusterIP": buildClusterIPValues(cfg),

			"tls": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
		},

		"externalURL": pulumi.String(
			"http://" + cfg.Network.RegistryHostname,
		),

		"harborAdminPassword": cfg.Harbor.AdminPassword,

		"updateStrategy": pulumi.Map{
			"type": pulumi.String("Recreate"),
		},

		"persistence": buildPersistenceValues(
			cfg.Storage,
		),

		"trivy": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
	}

	applyNodeSelector(
		values,
		cfg.Storage.NodeHostname,
	)

	return values
}

func buildClusterIPValues(cfg config.Config) pulumi.Map {
	values := pulumi.Map{
		"name": pulumi.String(ServiceName),

		"ports": pulumi.Map{
			"httpPort":  pulumi.Int(80),
			"httpsPort": pulumi.Int(443),
		},
	}

	if cfg.Network.StaticClusterIP != "" {
		values["staticClusterIP"] = pulumi.String(
			cfg.Network.StaticClusterIP,
		)
	}

	return values
}

func buildPersistenceValues(cfg config.Storage) pulumi.Map {
	pvc := pulumi.Map{
		"registry": pulumi.Map{
			"size": pulumi.String("100Gi"),
			"accessMode": pulumi.String(
				"ReadWriteOnce",
			),
		},

		"jobservice": pulumi.Map{
			"jobLog": pulumi.Map{
				"size": pulumi.String("1Gi"),
				"accessMode": pulumi.String(
					"ReadWriteOnce",
				),
			},
		},

		"database": pulumi.Map{
			"size": pulumi.String("2Gi"),
			"accessMode": pulumi.String(
				"ReadWriteOnce",
			),
		},

		"redis": pulumi.Map{
			"size": pulumi.String("1Gi"),
			"accessMode": pulumi.String(
				"ReadWriteOnce",
			),
		},
	}

	if cfg.StorageClass != "" {
		applyStorageClass(pvc, cfg.StorageClass)
	}

	return pulumi.Map{
		"enabled":        pulumi.Bool(true),
		"resourcePolicy": pulumi.String("keep"),

		"persistentVolumeClaim": pvc,
	}
}

func applyStorageClass(pvcs pulumi.Map, storageClass string) {
	class := pulumi.String(storageClass)

	for _, component := range []string{
		"registry",
		"database",
		"redis",
	} {
		values, ok := pvcs[component].(pulumi.Map)
		if !ok {
			continue
		}

		values["storageClass"] = class
	}

	jobservice, ok := pvcs["jobservice"].(pulumi.Map)
	if !ok {
		return
	}

	jobLog, ok := jobservice["jobLog"].(pulumi.Map)
	if !ok {
		return
	}

	jobLog["storageClass"] = class
}

func applyNodeSelector(values pulumi.Map, nodeHostname string) {
	if nodeHostname == "" {
		return
	}

	nodeSelector := pulumi.StringMap{
		"kubernetes.io/hostname": pulumi.String(
			nodeHostname,
		),
	}

	values["nodeSelector"] = nodeSelector

	for _, component := range []string{
		"core",
		"portal",
		"jobservice",
		"nginx",
		"registry",
		"database",
		"redis",
	} {
		values[component] = pulumi.Map{
			"nodeSelector": nodeSelector,
		}
	}
}
