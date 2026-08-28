package mirror

import (
	_ "embed"
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/config"
)

// mirrorScript is the shared skopeo digest-check/copy/failure-tracking
// implementation for all three mirror paths (public images, pinned GHCR
// images, discovered private GHCR images) — see scripts/mirror.sh.
//
//go:embed scripts/mirror.sh
var mirrorScript string

// discoverScript populates /images/private-images.txt for the private-image
// CronJob — see scripts/discover-private-images.sh.
//
//go:embed scripts/discover-private-images.sh
var discoverScript string

const (
	mirrorCredsSecretName     = "harbor-mirror-creds"
	mirrorNamespace           = "harbor-image-mirror"
	mirrorImagesConfigMapName = "harbor-mirror-images"
	mirrorPublicJobName       = "harbor-image-mirror-public"
	mirrorPinnedJobName       = "harbor-image-mirror-pinned"
	mirrorCronJobName         = "harbor-image-mirror"
)

func secretEnvVar(name, key string, optional bool) *corev1.EnvVarArgs {
	return &corev1.EnvVarArgs{
		Name: pulumi.String(name),
		ValueFrom: &corev1.EnvVarSourceArgs{
			SecretKeyRef: &corev1.SecretKeySelectorArgs{
				Name:     pulumi.String(mirrorCredsSecretName),
				Key:      pulumi.String(key),
				Optional: pulumi.Bool(optional),
			},
		},
	}
}

// Deploy creates the harbor-image-mirror namespace, a one-shot
// Job for the static public-image list, and — depending on
// harbor:ghcrSyncMode — either another one-shot Job (mode "pinned") or a
// recurring CronJob (mode "all"/"min-version") for GHCR private images.
//
// The split follows one rule: anything that's a fixed, operator-declared
// list (harbor:publicImages; harbor:ghcrPinnedImages in "pinned" mode) has
// nothing to "monitor" — it only needs re-syncing when that value itself
// changes, so it runs as a Job with ReplaceOnChanges. Anything that can
// change on its own without any stack config change (GHCR tags appearing
// under "all"/"min-version" mode) genuinely needs periodic polling, so it
// stays on the CronJob.
//
// Requires robotPassword (push auth always needs it, regardless of project
// visibility); GHCR credentials are optional — without them, private-image
// mirroring is skipped entirely, in every mode.
func Deploy(
	ctx *pulumi.Context,
	k8sProvider *kubernetes.Provider,
	cfg config.Mirror,
	registryHostname string,
	robotUsername string,
	robotPassword pulumi.StringOutput,
) error {
	providerOpt := pulumi.Provider(k8sProvider)

	ns, err := corev1.NewNamespace(
		ctx,
		"harbor-image-mirror-namespace",
		&corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(mirrorNamespace),
			},
		},
		providerOpt,
	)
	if err != nil {
		return fmt.Errorf("harbor-image-mirror namespace: %w", err)
	}

	cm, err := corev1.NewConfigMap(
		ctx,
		mirrorImagesConfigMapName,
		&corev1.ConfigMapArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(mirrorImagesConfigMapName),
				Namespace: pulumi.String(mirrorNamespace),
			},
			Data: pulumi.StringMap{
				// No built-in default — a stack that leaves this unset simply
				// mirrors nothing for min-version mode, same as unset GHCR
				// credentials skip private images entirely.
				//
				// ghcrPinnedImages isn't here: pinned mode doesn't use the
				// CronJob/discover step at all.
				"min-versions.txt": pulumi.String(
					cfg.GHCR.MinVersions,
				),
			},
		},
		providerOpt,
		pulumi.DependsOn([]pulumi.Resource{
			ns,
		}),
	)
	if err != nil {
		return fmt.Errorf(
			"harbor-mirror-images configmap: %w",
			err,
		)
	}

	secret, err := corev1.NewSecret(
		ctx,
		mirrorCredsSecretName,
		&corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(mirrorCredsSecretName),
				Namespace: pulumi.String(mirrorNamespace),
			},
			Type: pulumi.String("Opaque"),
			StringData: pulumi.StringMap{
				"robot-user":     pulumi.String(robotUsername),
				"robot-password": robotPassword,
				"ghcr-user":      pulumi.String(cfg.GHCR.User),
				"ghcr-token":     cfg.GHCR.Token,
			},
		},
		providerOpt,
		pulumi.DependsOn([]pulumi.Resource{
			ns,
		}),
	)
	if err != nil {
		return fmt.Errorf(
			"harbor-mirror-creds secret: %w",
			err,
		)
	}

	if err := deployPublicImagesJob(
		ctx,
		k8sProvider,
		cfg,
		registryHostname,
		ns,
		secret,
	); err != nil {
		return fmt.Errorf(
			"harbor-image-mirror-public job: %w",
			err,
		)
	}

	if cfg.GHCR.SyncMode == config.SyncModePinned {
		if err := deployPinnedImagesJob(
			ctx,
			k8sProvider,
			cfg,
			registryHostname,
			ns,
			secret,
		); err != nil {
			return fmt.Errorf(
				"harbor-image-mirror-pinned job: %w",
				err,
			)
		}
	} else {
		if err := deployPrivateImagesCronJob(
			ctx,
			k8sProvider,
			cfg,
			registryHostname,
			ns,
			cm,
			secret,
		); err != nil {
			return fmt.Errorf(
				"harbor-image-mirror cronjob: %w",
				err,
			)
		}
	}

	return nil
}

// deployPublicImagesJob mirrors harbor:publicImages via a plain Job, not a
// CronJob. The image list is embedded directly as an env var (not mounted
// from a ConfigMap) specifically so that editing harbor:publicImages changes
// this Job's pod spec — combined with ReplaceOnChanges below, that's what
// makes Pulumi delete-and-recreate (i.e. re-run) it exactly when the
// declared list changes, and leave it alone otherwise. A Job's
// spec.template is immutable in Kubernetes once created, so ReplaceOnChanges
// is required — an in-place update attempt would be rejected by the API.
//
// Public images need no GHCR auth, so MIRROR_REQUIRES_AUTH is left unset —
// mirror.sh falls back to anonymous source access.
func deployPublicImagesJob(
	ctx *pulumi.Context,
	k8sProvider *kubernetes.Provider,
	cfg config.Mirror,
	harborHost string,
	ns *corev1.Namespace,
	secret *corev1.Secret,
) error {
	providerOpt := pulumi.Provider(k8sProvider)

	_, err := batchv1.NewJob(
		ctx,
		mirrorPublicJobName,
		&batchv1.JobArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(mirrorPublicJobName),
				Namespace: pulumi.String(mirrorNamespace),
			},
			Spec: &batchv1.JobSpecArgs{
				BackoffLimit:            pulumi.Int(3),
				TtlSecondsAfterFinished: pulumi.Int(3600),
				Template: &corev1.PodTemplateSpecArgs{
					Spec: &corev1.PodSpecArgs{
						RestartPolicy: pulumi.String("Never"),
						Containers: corev1.ContainerArray{
							&corev1.ContainerArgs{
								Name:  pulumi.String("mirror"),
								Image: pulumi.String("quay.io/skopeo/stable:latest"),
								Env: corev1.EnvVarArray{
									&corev1.EnvVarArgs{
										Name:  pulumi.String("HARBOR_HOST"),
										Value: pulumi.String(harborHost),
									},
									&corev1.EnvVarArgs{
										Name:  pulumi.String("MIRROR_LIST"),
										Value: pulumi.String(cfg.PublicImages),
									},
									secretEnvVar(
										"HARBOR_ROBOT_USER",
										"robot-user",
										false,
									),
									secretEnvVar(
										"HARBOR_ROBOT_PASSWORD",
										"robot-password",
										false,
									),
								},
								Command: pulumi.StringArray{
									pulumi.String("bash"),
									pulumi.String("-c"),
								},
								Args: pulumi.StringArray{
									pulumi.String(mirrorScript),
								},
							},
						},
					},
				},
			},
		},
		providerOpt,
		pulumi.DependsOn([]pulumi.Resource{
			ns,
			secret,
		}),
		pulumi.ReplaceOnChanges([]string{
			"spec",
		}),
	)
	if err != nil {
		return err
	}

	return nil
}

// deployPinnedImagesJob mirrors harbor:ghcrPinnedImages via a plain Job, same
// reasoning and same ReplaceOnChanges mechanism as deployPublicImagesJob —
// an exact package:tag list is nothing to poll for, it only needs re-syncing
// when the list itself changes. Only called when harbor:ghcrSyncMode is
// "pinned"; "all"/"min-version" use deployPrivateImagesCronJob instead,
// since those genuinely need periodic GHCR polling.
func deployPinnedImagesJob(
	ctx *pulumi.Context,
	k8sProvider *kubernetes.Provider,
	cfg config.Mirror,
	harborHost string,
	ns *corev1.Namespace,
	secret *corev1.Secret,
) error {
	providerOpt := pulumi.Provider(k8sProvider)

	_, err := batchv1.NewJob(
		ctx,
		mirrorPinnedJobName,
		&batchv1.JobArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(mirrorPinnedJobName),
				Namespace: pulumi.String(mirrorNamespace),
			},
			Spec: &batchv1.JobSpecArgs{
				BackoffLimit:            pulumi.Int(3),
				TtlSecondsAfterFinished: pulumi.Int(3600),
				Template: &corev1.PodTemplateSpecArgs{
					Spec: &corev1.PodSpecArgs{
						RestartPolicy: pulumi.String("Never"),
						Containers: corev1.ContainerArray{
							&corev1.ContainerArgs{
								Name:  pulumi.String("mirror"),
								Image: pulumi.String("quay.io/skopeo/stable:latest"),
								Env: corev1.EnvVarArray{
									&corev1.EnvVarArgs{
										Name:  pulumi.String("HARBOR_HOST"),
										Value: pulumi.String(harborHost),
									},
									&corev1.EnvVarArgs{
										Name: pulumi.String(
											"MIRROR_LIST",
										),
										Value: pulumi.String(
											cfg.GHCR.PinnedImages,
										),
									},
									&corev1.EnvVarArgs{
										Name: pulumi.String(
											"MIRROR_REQUIRES_AUTH",
										),
										Value: pulumi.String("true"),
									},
									secretEnvVar(
										"HARBOR_ROBOT_USER",
										"robot-user",
										false,
									),
									secretEnvVar(
										"HARBOR_ROBOT_PASSWORD",
										"robot-password",
										false,
									),
									secretEnvVar(
										"GHCR_USER",
										"ghcr-user",
										true,
									),
									secretEnvVar(
										"GHCR_TOKEN",
										"ghcr-token",
										true,
									),
								},
								Command: pulumi.StringArray{
									pulumi.String("bash"),
									pulumi.String("-c"),
								},
								Args: pulumi.StringArray{
									pulumi.String(mirrorScript),
								},
							},
						},
					},
				},
			},
		},
		providerOpt,
		pulumi.DependsOn([]pulumi.Resource{
			ns,
			secret,
		}),
		pulumi.ReplaceOnChanges([]string{
			"spec",
		}),
	)
	if err != nil {
		return err
	}

	return nil
}

// deployPrivateImagesCronJob discovers and mirrors GHCR private images on a
// 5-minute recurring schedule — unlike the public list, private tags can
// appear without any stack config change, so this needs actual polling.
func deployPrivateImagesCronJob(
	ctx *pulumi.Context,
	k8sProvider *kubernetes.Provider,
	cfg config.Mirror,
	harborHost string,
	ns *corev1.Namespace,
	cm *corev1.ConfigMap,
	secret *corev1.Secret,
) error {
	providerOpt := pulumi.Provider(k8sProvider)

	staticAndDiscoveredMounts := corev1.VolumeMountArray{
		&corev1.VolumeMountArgs{
			Name:      pulumi.String("discovered"),
			MountPath: pulumi.String("/images"),
		},
		&corev1.VolumeMountArgs{
			Name:      pulumi.String("static"),
			MountPath: pulumi.String("/images-static"),
		},
	}

	_, err := batchv1.NewCronJob(
		ctx,
		mirrorCronJobName,
		&batchv1.CronJobArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(mirrorCronJobName),
				Namespace: pulumi.String(mirrorNamespace),
			},
			Spec: &batchv1.CronJobSpecArgs{
				Schedule:                   pulumi.String("*/5 * * * *"),
				ConcurrencyPolicy:          pulumi.String("Forbid"),
				SuccessfulJobsHistoryLimit: pulumi.Int(3),
				FailedJobsHistoryLimit:     pulumi.Int(3),
				JobTemplate: &batchv1.JobTemplateSpecArgs{
					Spec: &batchv1.JobSpecArgs{
						BackoffLimit: pulumi.Int(1),
						Template: &corev1.PodTemplateSpecArgs{
							Spec: &corev1.PodSpecArgs{
								RestartPolicy: pulumi.String("Never"),

								InitContainers: corev1.ContainerArray{
									&corev1.ContainerArgs{
										Name:  pulumi.String("discover"),
										Image: pulumi.String("alpine:3.20"),
										Env: corev1.EnvVarArray{
											&corev1.EnvVarArgs{
												Name: pulumi.String(
													"GHCR_ORG",
												),
												Value: pulumi.String(
													cfg.GHCR.Org,
												),
											},
											&corev1.EnvVarArgs{
												Name: pulumi.String(
													"GHCR_SYNC_MODE",
												),
												Value: pulumi.String(
													string(cfg.GHCR.SyncMode),
												),
											},
											secretEnvVar(
												"GHCR_TOKEN",
												"ghcr-token",
												true,
											),
										},
										Command: pulumi.StringArray{
											pulumi.String("sh"),
											pulumi.String("-c"),
										},
										Args: pulumi.StringArray{
											pulumi.String(discoverScript),
										},
										VolumeMounts: staticAndDiscoveredMounts,
									},
								},

								Containers: corev1.ContainerArray{
									&corev1.ContainerArgs{
										Name:  pulumi.String("mirror"),
										Image: pulumi.String("quay.io/skopeo/stable:latest"),
										Env: corev1.EnvVarArray{
											&corev1.EnvVarArgs{
												Name: pulumi.String(
													"HARBOR_HOST",
												),
												Value: pulumi.String(
													harborHost,
												),
											},
											&corev1.EnvVarArgs{
												Name: pulumi.String(
													"MIRROR_LIST_FILE",
												),
												Value: pulumi.String(
													"/images/private-images.txt",
												),
											},
											&corev1.EnvVarArgs{
												Name: pulumi.String(
													"MIRROR_REQUIRES_AUTH",
												),
												Value: pulumi.String(
													"true",
												),
											},
											secretEnvVar(
												"HARBOR_ROBOT_USER",
												"robot-user",
												false,
											),
											secretEnvVar(
												"HARBOR_ROBOT_PASSWORD",
												"robot-password",
												false,
											),
											secretEnvVar(
												"GHCR_USER",
												"ghcr-user",
												true,
											),
											secretEnvVar(
												"GHCR_TOKEN",
												"ghcr-token",
												true,
											),
										},
										Command: pulumi.StringArray{
											pulumi.String("bash"),
											pulumi.String("-c"),
										},
										Args: pulumi.StringArray{
											pulumi.String(mirrorScript),
										},
										VolumeMounts: staticAndDiscoveredMounts,
									},
								},

								Volumes: corev1.VolumeArray{
									&corev1.VolumeArgs{
										Name: pulumi.String(
											"discovered",
										),
										EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
									},
									&corev1.VolumeArgs{
										Name: pulumi.String(
											"static",
										),
										ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
											Name: pulumi.String(
												mirrorImagesConfigMapName,
											),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		providerOpt,
		pulumi.DependsOn([]pulumi.Resource{
			ns,
			cm,
			secret,
		}),
	)
	if err != nil {
		return err
	}

	return nil
}
