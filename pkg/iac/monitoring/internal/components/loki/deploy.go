package loki

import (
	"fmt"

	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/monitoring/internal/config"

	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(ctx *pulumi.Context, cfg appconfig.Config, opts ...pulumi.ResourceOption) error {
	job, err := createBucketJob(ctx, cfg, opts...)
	if err != nil {
		return err
	}

	releaseOpts := append(opts, pulumi.DependsOn([]pulumi.Resource{job}))

	values := pulumi.Map{
		"deploymentMode": pulumi.String("Monolithic"),
		"loki": pulumi.Map{
			"auth_enabled": pulumi.Bool(false),
			"analytics": pulumi.Map{
				"reporting_enabled": pulumi.Bool(false),
			},
			"commonConfig": pulumi.Map{
				"replication_factor": pulumi.Int(1),
			},
			"storage": pulumi.Map{
				"type": pulumi.String("s3"),
				"bucketNames": pulumi.Map{
					"chunks": pulumi.String(cfg.Loki.S3Bucket),
					"ruler":  pulumi.String(cfg.Loki.S3Bucket),
				},
				"s3": pulumi.Map{
					"endpoint":         pulumi.String(cfg.S3.Endpoint),
					"region":           pulumi.String("us-east-1"),
					"accessKeyId":      pulumi.String(cfg.S3.User),
					"secretAccessKey":  cfg.S3.Password,
					"s3ForcePathStyle": pulumi.Bool(true),
					"insecure":         pulumi.Bool(true),
				},
			},
			"schemaConfig": pulumi.Map{
				"configs": pulumi.Array{
					pulumi.Map{
						"from":         pulumi.String("2024-01-01"),
						"store":        pulumi.String("tsdb"),
						"object_store": pulumi.String("s3"),
						"schema":       pulumi.String("v13"),
						"index": pulumi.Map{
							"prefix": pulumi.String("index_"),
							"period": pulumi.String("24h"),
						},
					},
				},
			},
			"compactor": pulumi.Map{
				"working_directory":             pulumi.String("/var/loki/compactor"),
				"delete_request_store":          pulumi.String("s3"),
				"retention_enabled":             pulumi.Bool(true),
				"retention_delete_delay":        pulumi.String("2h"),
				"retention_delete_worker_count": pulumi.Int(150),
			},
			"limits_config": pulumi.Map{
				"retention_period":            pulumi.String("336h"),
				"ingestion_rate_mb":           pulumi.Int(16),
				"ingestion_burst_size_mb":     pulumi.Int(32),
				"per_stream_rate_limit":       pulumi.String("3MB"),
				"per_stream_rate_limit_burst": pulumi.String("10MB"),
				"max_entries_limit_per_query": pulumi.Int(10000),
				"max_query_length":            pulumi.String("336h"),
				"max_query_parallelism":       pulumi.Int(4),
			},
			"chunk_store_config": pulumi.Map{
				"chunk_encoding": pulumi.String("zstd"),
			},
		},
		"singleBinary": pulumi.Map{
			"replicas":    pulumi.Int(1),
			"persistence": lokiPersistence(cfg),
			"resources": pulumi.Map{
				"requests": pulumi.Map{"cpu": pulumi.String("250m"), "memory": pulumi.String("256Mi")},
				"limits":   pulumi.Map{"cpu": pulumi.String("1"), "memory": pulumi.String("1Gi")},
			},
		},
		"read": pulumi.Map{
			"replicas": pulumi.Int(0),
		},
		"write": pulumi.Map{
			"replicas": pulumi.Int(0),
		},
		"backend": pulumi.Map{
			"replicas": pulumi.Int(0),
		},
		// The chart's memcached-based chunks/results caches are enabled by
		// default and sized for production-scale clusters (chunksCache alone
		// requests ~9.6Gi). Unaddressed by singleBinary.resources above since
		// these are separate top-level StatefulSets — left enabled, the
		// chunks-cache pod stays Pending on a size-constrained dev node,
		// hanging the Helm release until pulumi up times out. Not needed at
		// this log volume (see Monolithic mode rationale below).
		"chunksCache": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		"resultsCache": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
	}

	_, err = helmv3.NewRelease(ctx, "loki", &helmv3.ReleaseArgs{
		Name:        pulumi.String("loki"),
		Chart:       pulumi.String(cfg.Loki.ChartPath),
		Namespace:   pulumi.String(cfg.Namespace),
		Values:      values,
		WaitForJobs: pulumi.Bool(true),
		Timeout:     pulumi.Int(300),
	}, releaseOpts...)
	return err
}

func lokiPersistence(cfg appconfig.Config) pulumi.Map {
	m := pulumi.Map{
		"enabled": pulumi.Bool(true),
		"size":    pulumi.String("10Gi"),
	}
	if cfg.Loki.StorageClass != "" {
		m["storageClassName"] = pulumi.String(cfg.Loki.StorageClass)
	}
	return m
}

// createBucketJob creates a one-shot Kubernetes Job that creates the Loki S3
// bucket in RustFS before Loki starts. Uses "head-bucket || create-bucket"
// rather than a single one-shot idempotent create call — mc's "mb
// --ignore-existing" doesn't reliably no-op against RustFS's S3 API
// responses, which left the Job failing with BackoffLimitExceeded even when
// the bucket already existed. aws-cli's head-bucket/create-bucket are plain
// S3 API calls any S3-compatible backend (including RustFS) implements the
// same way, so this is portable across backends, not just AWS.
func createBucketJob(ctx *pulumi.Context, cfg appconfig.Config, opts ...pulumi.ResourceOption) (*batchv1.Job, error) {
	cmd := fmt.Sprintf(
		"aws --endpoint-url %s s3api head-bucket --bucket %s || aws --endpoint-url %s s3api create-bucket --bucket %s",
		cfg.S3.Endpoint, cfg.Loki.S3Bucket, cfg.S3.Endpoint, cfg.Loki.S3Bucket,
	)

	return batchv1.NewJob(ctx, "loki-create-bucket", &batchv1.JobArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("loki-create-bucket"),
			Namespace: pulumi.String(cfg.Namespace),
		},
		Spec: &batchv1.JobSpecArgs{
			BackoffLimit: pulumi.Int(5),
			Template: &corev1.PodTemplateSpecArgs{
				Spec: &corev1.PodSpecArgs{
					RestartPolicy: pulumi.String("OnFailure"),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:    pulumi.String("s3-client"),
							Image:   pulumi.String(cfg.Loki.S3ClientImage),
							Command: pulumi.StringArray{pulumi.String("sh"), pulumi.String("-c")},
							Args:    pulumi.StringArray{pulumi.String(cmd)},
							Env: corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name:  pulumi.String("AWS_ACCESS_KEY_ID"),
									Value: pulumi.String(cfg.S3.User),
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("AWS_SECRET_ACCESS_KEY"),
									Value: cfg.S3.Password,
								},
								&corev1.EnvVarArgs{
									// RustFS ignores the region value but aws-cli
									// requires one to be set to run at all.
									Name:  pulumi.String("AWS_DEFAULT_REGION"),
									Value: pulumi.String("us-east-1"),
								},
							},
						},
					},
				},
			},
		},
	}, opts...)
}
