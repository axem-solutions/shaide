package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
			// {
			// 	Name:    "set up Harbor",
			// 	Run:     setupHarbor,
			// 	When:    InstallMode,
			// 	Recover: recoverScrapeHarbor,
			// },
			// The Harbor stack only creates the harbor-pull-secret when
			// harbor:robotPassword is set, and that password is not known until
			// "set up Harbor" has prompted for it and created the robot account.
			// The first deploy above therefore leaves the secret out, and this
			// second pass adds it — the two-phase sequence the pull secret block
			// in pkg/iac/harbor documents.
			// {
			// 	Name:    "redeploy Harbor with robot credentials",
			// 	Run:     stacks.DeployHarbor,
			// 	Recover: pulumi.RecoverHarbor,
			// 	When:    InstallCloud,
			// },
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

func setupHarbor(rt *core.Runtime) error {
	label := "Harbor robot password"
	var robotPassword string
	for {
		value, err := rt.Reporter.Input(label, "", "")
		if err != nil {
			return err
		}

		value = strings.TrimSpace(value)
		if value != "" {
			robotPassword = value
			break
		}

		rt.Detailf("%s is required", label)
	}

	// The deadline covers the Harbor API calls below and starts once the
	// operator has answered, so time spent at the prompt does not consume it.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	rt.Discovery.RobotPassword = robotPassword

	rt.Discovery.Auth = auth.Credentials{
		Registry: fmt.Sprintf("harbor.%s.svc.cluster.local", rt.Bootstrap.Config.Harbor.Namespace),
		Username: "admin",
		Password: rt.Discovery.AdminPassword,
	}

	rt.Detailf("%s", rt.Discovery.Auth.Registry)

	// On the fresh-install path CheckResources fails before EnsurePortForward
	// runs, so the port-forward must be established here before building the
	// Harbor client.
	//
	// Only the tunnel is opened at this point. The registry readiness probe
	// requests a repository-scoped token for a Catalog repo, and on a fresh
	// install its project does not exist yet — Harbor answers 401 for an
	// unknown project rather than 404, which the probe would retry until its
	// budget expired. So create the projects and the robot account first, then
	// probe.
	if err := startPortForward(rt); err != nil {
		return err
	}

	rt.Discovery.Client = harborapi.NewClient(rt.Discovery.HarborForward.Address(), rt.Discovery.Auth)

	if err := ensureHarborProjects(ctx, rt.Discovery.Client, rt.Bootstrap.Config.Harbor.Projects); err != nil {
		return err
	}

	if err := ensureHarborRobotCredentials(ctx, rt.Discovery.Client, robotPassword); err != nil {
		return err
	}
	rt.Detailf("harbor projects and robot account are ready")

	// Projects now exist, so a repository-scoped token can be issued and the
	// probe distinguishes a warming-up token service from a real auth problem.
	if err := waitRegistryReady(rt); err != nil {
		return err
	}

	return nil
}

func ensureHarborProjects(ctx context.Context, client *harborapi.Client, projects []string) error {
	for _, project := range projects {
		if project == "" {
			return fmt.Errorf("harbor project name is empty")
		}

		if err := ensureHarborProject(ctx, client, project); err != nil {
			return err
		}
	}

	return nil
}

func ensureHarborProject(ctx context.Context, client *harborapi.Client, name string) error {
	projects, err := harborapi.ListProjects(ctx, client)
	if err != nil {
		return fmt.Errorf("list Harbor projects: %w", err)
	}

	for _, project := range projects {
		if project.Name == name {
			// Always asserted, not just checked — self-heals a project that
			// was made private some other way, same as resetHarborRobotPassword
			// always PATCHes the robot's password below regardless of whether
			// the account already exists.
			if err := harborapi.UpdateProjectVisibility(ctx, client, name, true); err != nil {
				return fmt.Errorf("make Harbor project %q public: %w", name, err)
			}
			return nil
		}
	}

	if err := harborapi.CreateProject(ctx, client, harborapi.CreateProjectRequest{
		Name:   name,
		Public: true,
	}); err != nil {
		return fmt.Errorf("create Harbor project %q: %w", name, err)
	}

	return nil
}

func ensureHarborRobotCredentials(ctx context.Context, client *harborapi.Client, password string) error {
	robot, err := ensureHarborRobotAccount(ctx, client)
	if err != nil {
		return err
	}

	if err := resetHarborRobotPassword(ctx, client, robot.ID, password); err != nil {
		return err
	}

	if err := validateHarborRobotToken(ctx, client, password); err != nil {
		return fmt.Errorf("validate Harbor robot credentials: %w", err)
	}

	return nil
}

func ensureHarborRobotAccount(ctx context.Context, client *harborapi.Client) (harborapi.RobotAccount, error) {
	robots, err := harborapi.ListRobotAccounts(ctx, client)
	if err != nil {
		return harborapi.RobotAccount{}, fmt.Errorf("list Harbor robot accounts: %w", err)
	}

	for _, robot := range robots {
		if robot.Name == config.DefaultHarborRobotName || robot.Name == config.DefaultHarborRobotFullName {
			return robot, nil
		}
	}

	robot, err := harborapi.CreateRobotAccount(ctx, client, harborapi.CreateRobotAccountRequest{
		Name:     config.DefaultHarborRobotName,
		Level:    "system",
		Duration: -1,
		Permissions: []harborapi.RobotPermission{
			{
				Kind:      "project",
				Namespace: "*",
				Access: []harborapi.RobotAccess{
					{Action: "pull", Resource: "repository"},
					{Action: "push", Resource: "repository"},
					{Action: "delete", Resource: "repository"},
					{Action: "list", Resource: "repository"},

					{Action: "read", Resource: "artifact"},
					{Action: "list", Resource: "artifact"},
				},
			},
		},
	})
	if err != nil {
		return harborapi.RobotAccount{}, fmt.Errorf("create Harbor robot account: %w", err)
	}

	return robot, nil
}

func resetHarborRobotPassword(ctx context.Context, client *harborapi.Client, robotID int64, password string) error {
	if password == "" {
		return fmt.Errorf("harbor robot password is empty")
	}

	if err := harborapi.SetRobotPassword(
		ctx,
		client,
		robotID,
		harborapi.SetRobotPasswordRequest{
			Secret: password,
		},
	); err != nil {
		return fmt.Errorf("set Harbor robot password: %w", err)
	}

	return nil
}

func validateHarborRobotToken(ctx context.Context, client *harborapi.Client, password string) error {
	requestURL := *client.Http.BaseURL
	requestURL.Path = "/service/token"

	q := requestURL.Query()
	q.Set("service", "harbor-registry")
	requestURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}

	req.SetBasicAuth(config.DefaultHarborRobotFullName, password)

	resp, err := client.Http.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request Harbor token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		return fmt.Errorf(
			"token service returned HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	return nil
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
