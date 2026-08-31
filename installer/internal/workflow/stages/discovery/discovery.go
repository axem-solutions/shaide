package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/config"
	harborapi "github.com/axem-solutions/ai_platform/installer/internal/harbor/api"
	"github.com/axem-solutions/ai_platform/installer/internal/harbor/auth"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	orasapi "github.com/axem-solutions/ai_platform/installer/internal/oras/client"
	"github.com/axem-solutions/ai_platform/installer/internal/preloader"
	"github.com/axem-solutions/ai_platform/installer/internal/progress"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/pulumi"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/stages/pulumi/stacks"
	pkgkube "github.com/axem-solutions/ai_platform/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

func Stage() core.Stage {
	return core.Stage{
		Name: "discovery",
		Steps: []core.Step{
			{
				Name:    "check existing Harbor",
				Run:     CheckResources,
				Recover: recoverCheckResources,
			},
			// Harbor is deployed one of two ways, selected by the platform
			// detected in the initK8s stage.
			//
			// On-prem (air-gapped): nodes have no route to a public registry,
			// so Harbor's images are side-loaded into containerd over SSH and
			// Harbor is deployed from the on-prem services stack.
			{
				Name:    "preload Harbor Images",
				Run:     preloadHarbor,
				When:    InstallOnPrem,
				Recover: recoverPreloadHarbor,
			},
			// side-loading, and nothing to SSH into.
			//
			// Both paths converge here: the Harbor project selects its storage
			// behaviour from the configured platform, so one deploy step
			// serves on-prem and cloud alike.
			{
				Name:    "deploy Harbor",
				Run:     stacks.DeployHarbor,
				Recover: pulumi.RecoverHarbor,
			},
			{
				Name: "check Harbor target",
				Run:  checkHarborTarget,
				When: InstallMode,
			},
			{
				Name:    "check Harbor pull secret",
				Run:     checkHarborPullSecret, // fills rt.Discovery.Auth
				When:    InstallMode,
				Recover: recoverCheckResources,
			},
		},
	}
}

func InstallMode(rt *core.Runtime) bool {
	return rt.Discovery.Mode == core.Install
}

// InstallCloud gates steps that only apply to a managed cloud cluster, where
// nodes can pull from public registries.
func InstallCloud(rt *core.Runtime) bool {
	return InstallMode(rt) && kube.IsCloud(rt.Bootstrap.CloudPlatform)
}

// InstallOnPrem gates steps that only apply to an air-gapped on-prem cluster,
// where images have to be side-loaded onto the nodes.
func InstallOnPrem(rt *core.Runtime) bool {
	return InstallMode(rt) && !kube.IsCloud(rt.Bootstrap.CloudPlatform)
}

func CheckResources(rt *core.Runtime) error {
	if err := checkHarborTarget(rt); err != nil {
		return err
	}

	if err := checkHarborPullSecret(rt); err != nil {
		return err
	}

	if err := EnsurePortForward(rt); err != nil {
		return err
	}

	rt.Discovery.Client = harborapi.NewClient(
		rt.Discovery.HarborForward.Address(),
		rt.Discovery.Auth,
	)

	rt.Discovery.Mode = core.Update
	rt.Detailf("harbor client ready for %s", rt.Discovery.HarborForward.Address())

	return nil
}

func checkHarborTarget(rt *core.Runtime) error {
	reqCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ns := rt.Bootstrap.Config.Harbor.Namespace
	svcName := rt.Bootstrap.Config.Harbor.Service

	found, err := kube.SearchNamespace(reqCtx, rt.Cluster.Client, ns)
	if err != nil {
		return err
	}

	if !found {
		return harborDiscoveryError{
			Kind:         harborNamespace,
			Problem:      missing,
			ResourceName: ns,
		}
	}

	svc := pkgkube.NewServiceRef(reqCtx, rt.Cluster.Client, ns, svcName)
	target, err := svc.ForwardTarget()
	if err != nil {
		return harborDiscoveryError{
			Kind:         harborService,
			Problem:      invalid,
			Namespace:    ns,
			ResourceName: svcName,
			Err:          err,
		}
	}

	rt.Discovery.Target = target
	return nil
}

func checkHarborPullSecret(rt *core.Runtime) error {
	reqCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ns := rt.Bootstrap.Config.Harbor.Namespace
	secretName := rt.Bootstrap.Config.Harbor.PullSecret

	auth, err := getAuthTokenFromSecret(reqCtx, rt.Cluster.Client, ns, secretName)
	if err != nil {
		return harborDiscoveryError{
			Kind:         harborSecret,
			Problem:      invalid,
			Namespace:    ns,
			ResourceName: secretName,
			Err:          err,
		}
	}

	rt.Discovery.Auth = auth
	return nil
}

func getAuthTokenFromSecret(ctx context.Context, clientset kubernetes.Interface, namespace, secretName string) (auth.Credentials, error) {
	raw, err := kube.ReadSecretKey(ctx, clientset, namespace, secretName, corev1.DockerConfigJsonKey)
	if err != nil {
		return auth.Credentials{}, err
	}
	creds, err := auth.ParseDockerCredentials(raw)
	if err != nil {
		return auth.Credentials{}, err
	}
	return creds, nil
}

const (
	defaultPreloaderCtrPath          = "/usr/local/bin/ctr"
	defaultPreloaderContainerdSocket = "/run/containerd/containerd.sock"
)

func preloadHarbor(rt *core.Runtime) error {
	// Second line of defence behind the InstallOnPrem gate. Preloading pushes
	// the images listed under goharbor_images into a node's containerd; with an
	// empty list there is nothing to push, and prompting for SSH details would
	// block the install on input that cannot be used. Note the check has to sit
	// here rather than inside preloader.Preload: that reaches its own
	// len(images)==0 guard only after remoteHealthCheck has already SSH'd.
	if len(rt.Bootstrap.Catalog.HarborImages) == 0 {
		rt.Detailf("no goharbor_images in the Catalog manifest, skipping Harbor image preload")
		return nil
	}

	opts, err := promptPreloaderOptions(rt)
	if err != nil {
		return err
	}

	preloader, err := preloader.NewPreloader(opts)
	if err != nil {
		return err
	}
	defer preloader.Close()

	if err := preloader.Preload(context.Background(), rt.Bootstrap.Catalog.HarborImages); err != nil {
		return err
	}
	return nil
}

func promptPreloaderOptions(rt *core.Runtime) (preloader.PreloaderOptions, error) {
	host, err := promptNonEmpty(rt, "Harbor node IP or hostname", "")
	if err != nil {
		return preloader.PreloaderOptions{}, err
	}

	user, err := promptNonEmpty(rt, "Harbor node SSH user", "")
	if err != nil {
		return preloader.PreloaderOptions{}, err
	}

	ctrPath, err := promptWithDefault(rt, "Remote ctr path", defaultPreloaderCtrPath)
	if err != nil {
		return preloader.PreloaderOptions{}, err
	}

	containerdSocket, err := promptWithDefault(
		rt,
		"Remote containerd socket",
		defaultPreloaderContainerdSocket,
	)
	if err != nil {
		return preloader.PreloaderOptions{}, err
	}

	if rt.Bootstrap.Config.Preloader.PrivateKeyFile == "" {
		return preloader.PreloaderOptions{}, fmt.Errorf(
			"%s is required for Harbor image preload",
			config.PrivateKeyPathEnv,
		)
	}

	return preloader.PreloaderOptions{
		Host:             host,
		User:             user,
		PrivateKeyPath:   rt.Bootstrap.Config.Preloader.PrivateKeyFile,
		LocalImageDir:    rt.Bootstrap.Catalog.ImagesDir,
		CtrPath:          ctrPath,
		Port:             rt.Bootstrap.Config.Preloader.SSHPort,
		ContainerdSocket: containerdSocket,
		LocalDir:         rt.Bootstrap.Catalog.ImagesDir,
		Progressf: func(e progress.Event) {
			rt.Reporter.ProgressModel(core.ModelProgress{
				ID:         fmt.Sprintf("%s\n%s", e.Phase, e.Current),
				Bytes:      e.Bytes,
				TotalBytes: e.TotalBytes,
				Files:      e.Files,
				TotalFiles: e.TotalFiles,
				Percent:    e.Percent,
				Done:       e.Done,
			})
		},
		Logf: rt.Detailf,
	}, nil
}

func promptNonEmpty(rt *core.Runtime, title, defaultValue string) (string, error) {
	for {
		value, err := rt.Reporter.Input(title, defaultValue, defaultValue)
		if err != nil {
			return "", err
		}

		value = strings.TrimSpace(value)
		if value != "" {
			return value, nil
		}

		rt.Detailf("%s is required", title)
	}
}

func promptWithDefault(rt *core.Runtime, title, defaultValue string) (string, error) {
	value, err := rt.Reporter.Input(title, defaultValue, defaultValue)
	if err != nil {
		return "", err
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}

	return value, nil
}

// EnsurePortForward opens the Harbor port-forward and waits for the registry
// token service to answer a repository-scoped request.
//
// Callers that run before the Harbor projects exist must not use this: the
// readiness probe asks for a scope Harbor cannot issue a token for yet. Use
// startPortForward, create the projects, then call waitRegistryReady.
func EnsurePortForward(rt *core.Runtime) error {
	if err := startPortForward(rt); err != nil {
		return err
	}

	// The TCP tunnel is up, but Harbor's registry token service may still be
	// warming up (especially right after a harbor-core restart). Wait until the
	// authenticated registry token exchange succeeds so the first manifest check
	// does not surface a transient 401 as a (misleading) credentials error.
	return waitRegistryReady(rt)
}

// startPortForward opens the tunnel to Harbor without probing the registry.
func startPortForward(rt *core.Runtime) error {
	if rt.Discovery.HarborForward != nil && rt.Discovery.Client != nil {
		return nil
	}

	if rt.Discovery.Target == nil {
		return fmt.Errorf("harbor forward target is not available")
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	forward, err := pkgkube.StartPortForward(
		reqCtx,
		rt.Cluster.RESTConfig,
		rt.Cluster.Client,
		pkgkube.ForwardRequest{
			Namespace:  rt.Bootstrap.Config.Harbor.Namespace,
			Service:    rt.Bootstrap.Config.Harbor.Service,
			LocalPort:  rt.Bootstrap.Config.Harbor.LocalPort,
			RemotePort: rt.Discovery.Target.TargetPort,
			PodName:    rt.Discovery.Target.PodName,
		},
	)
	if err != nil {
		return fmt.Errorf("start harbor port-forward: %w", err)
	}

	rt.Discovery.HarborForward = forward

	rt.Detailf("harbor port-forward open at %s", forward.Address())

	return nil
}

// Harbor registry readiness tuning. The probe exercises the full token
// exchange, so each attempt must tolerate Harbor's internal retries while the
// overall budget stays generous enough for a freshly (re)deployed Harbor to
// finish warming up its core/token service.
const (
	registryReadyBudget     = 120 * time.Second
	registryReadyPerAttempt = 8 * time.Second
	registryReadyBackoff    = 3 * time.Second
)

// waitRegistryReady blocks until the registry's authenticated /v2/ token
// exchange succeeds, or returns an actionable error once the budget is spent.
// Each attempt gets its own timeout so a single slow/hung token fetch cannot
// consume the whole budget (the earlier shared-context version effectively
// probed only once). A transient warmup is absorbed silently; a Harbor whose
// token service is genuinely down (e.g. harbor-core crashlooping because its
// zonal redis disk is stranded) fails here with a clear message instead of
// surfacing later as a misleading "wrong credentials" prompt.
func waitRegistryReady(rt *core.Runtime) error {
	if rt.Discovery.Auth.Username == "" {
		// No credentials to probe with (unexpected on both paths). Skip rather
		// than block; the downstream call will surface any real auth problem.
		return nil
	}

	// Probe with a real Catalog repo so the token request carries a repository
	// scope the robot account actually holds. A scopeless /v2/ probe would
	// always 401 for a repository-scoped robot and never report ready.
	models := rt.Bootstrap.Catalog.Models
	if len(models) == 0 {
		return nil
	}
	probeModel := models[0]

	probe := orasapi.NewClient(orasapi.ClientOptions{
		Registry: fmt.Sprintf("127.0.0.1:%d", rt.Discovery.HarborForward.LocalPort()),
		TargetCredentials: orasapi.Credential{
			Username: rt.Discovery.Auth.Username,
			Password: rt.Discovery.Auth.Password,
		},
	})

	deadline := time.Now().Add(registryReadyBudget)
	var lastErr error
	for attempt := 1; ; attempt++ {
		attemptCtx, cancel := context.WithTimeout(context.Background(), registryReadyPerAttempt)
		err := probe.Ping(attemptCtx, probeModel.HarborProject, probeModel.HarborName, probeModel.HarborTag)
		cancel()
		if err == nil {
			if attempt > 1 {
				rt.Detailf("harbor registry ready after %d attempts", attempt)
			}
			return nil
		}
		lastErr = err

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"harbor registry not ready: token service did not become available within %s "+
					"(check 'kubectl -n %s get pods'): %w",
				registryReadyBudget, rt.Bootstrap.Config.Harbor.Namespace, lastErr,
			)
		}
		rt.Detailf("harbor registry not ready (attempt %d): %v; retrying", attempt, lastErr)
		time.Sleep(registryReadyBackoff)
	}
}

func RefreshPortForward(rt *core.Runtime) error {
	if rt.Discovery.HarborForward != nil {
		rt.Discovery.HarborForward.Close()
		rt.Discovery.HarborForward = nil
	}
	rt.Discovery.Client = nil

	if rt.Discovery.Target == nil {
		if err := checkHarborTarget(rt); err != nil {
			return err
		}
	}

	if err := EnsurePortForward(rt); err != nil {
		return err
	}

	rt.Discovery.Client = harborapi.NewClient(
		rt.Discovery.HarborForward.Address(),
		rt.Discovery.Auth,
	)

	rt.Detailf("harbor port-forward refreshed at %s", rt.Discovery.HarborForward.Address())
	return nil
}
