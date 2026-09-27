package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	harborapi "github.com/axem-solutions/ai_platform/installer/internal/harbor/api"
	"github.com/axem-solutions/ai_platform/installer/internal/harbor/auth"
	"github.com/axem-solutions/ai_platform/installer/internal/httpapi"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

const (
	harborAdminUsername = "admin"

	// The Harbor chart stores the admin password here. A Harbor the installer
	// did not deploy has no Pulumi state to read it from.
	harborCoreSecret         = "harbor-core"
	harborAdminPasswordKey   = "HARBOR_ADMIN_PASSWORD"
	harborAdminPromptRetries = 3
)

// RequiredHarborProjects lists the projects this run pushes into: the ones the
// Harbor stack creates, plus every project the image and model manifests name.
func RequiredHarborProjects(rt *core.Runtime) []string {
	seen := map[string]struct{}{}
	add := func(name string) {
		if name = strings.TrimSpace(name); name != "" {
			seen[name] = struct{}{}
		}
	}

	for _, project := range rt.Bootstrap.Config.Harbor.Projects {
		add(project)
	}
	for _, image := range rt.Bootstrap.Catalog.ServiceImages {
		add(image.Project)
	}
	for _, model := range rt.Bootstrap.Catalog.Models {
		add(model.HarborProject)
	}

	projects := make([]string, 0, len(seen))
	for project := range seen {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	return projects
}

// HarborProjectForTarget maps an upload target (an image or model name) back to
// the Harbor project it is pushed into. It returns "" for an unknown target.
func HarborProjectForTarget(rt *core.Runtime, target string) string {
	for _, image := range rt.Bootstrap.Catalog.ServiceImages {
		if image.Name == target {
			return image.Project
		}
	}
	for _, model := range rt.Bootstrap.Catalog.Models {
		if model.HarborName == target {
			return model.HarborProject
		}
	}
	return ""
}

// EnsureHarborProjects creates the projects this run pushes into that Harbor
// does not have yet.
//
// The Harbor stack creates them on a fresh install, but an existing Harbor the
// installer did not deploy may lack them. Harbor answers a push into a missing
// project with 401, which reads as bad credentials, so they are created up
// front. Projects are created public, like the Harbor stack does. The robot
// account the installer uses is scoped to every project, so a new project needs
// no further permission.
func EnsureHarborProjects(rt *core.Runtime) error {
	address, err := harborForwardAddress(rt)
	if err != nil {
		return err
	}
	return ensureHarborProjectsAt(rt, address)
}

func ensureHarborProjectsAt(rt *core.Runtime, address string) error {
	required := RequiredHarborProjects(rt)
	if len(required) == 0 {
		return nil
	}

	// The robot usually sees every project, so the common case of nothing
	// missing needs no admin credentials at all.
	robot := harborapi.NewClient(address, rt.Discovery.Auth)
	if existing, err := projectNames(robot); err == nil {
		if len(missingProjects(required, existing)) == 0 {
			return nil
		}
	}

	admin, existing, err := harborAdminClient(rt, address)
	if err != nil {
		return err
	}

	for _, project := range missingProjects(required, existing) {
		if err := createPublicProject(admin, project); err != nil {
			return err
		}
		rt.Detailf("created missing Harbor project %q", project)
	}
	return nil
}

// HarborProjectExists reports whether Harbor has the project. ok is false when
// the robot cannot list projects, in which case nothing can be concluded.
func HarborProjectExists(rt *core.Runtime, project string) (exists bool, ok bool) {
	address, err := harborForwardAddress(rt)
	if err != nil {
		return false, false
	}
	return harborProjectExistsAt(rt, address, project)
}

func harborProjectExistsAt(rt *core.Runtime, address, project string) (exists bool, ok bool) {
	existing, err := projectNames(harborapi.NewClient(address, rt.Discovery.Auth))
	if err != nil {
		return false, false
	}
	_, exists = existing[project]
	return exists, true
}

func harborForwardAddress(rt *core.Runtime) (string, error) {
	if rt.Discovery.HarborForward == nil {
		return "", fmt.Errorf("harbor port-forward is not initialized")
	}
	return rt.Discovery.HarborForward.Address(), nil
}

// harborAdminClient returns a client authenticated as the Harbor admin, with
// the projects that client sees. The password comes from this run, from the
// Harbor chart's secret, or from the operator, in that order.
func harborAdminClient(rt *core.Runtime, address string) (*harborapi.Client, map[string]struct{}, error) {
	password := rt.Discovery.AdminPassword
	if password == "" {
		password = harborAdminPasswordFromSecret(rt)
	}

	for attempt := 0; attempt <= harborAdminPromptRetries; attempt++ {
		if password == "" {
			var err error
			password, err = rt.Reporter.Input("Harbor admin password", "", "")
			if err != nil {
				return nil, nil, err
			}
		}

		client := harborapi.NewClient(address, auth.Credentials{
			Username: harborAdminUsername,
			Password: password,
		})
		existing, err := projectNames(client)
		if err == nil {
			rt.Discovery.AdminPassword = password
			return client, existing, nil
		}
		if httpapi.ClassifyError(err) != httpapi.ErrAuth {
			return nil, nil, fmt.Errorf("list Harbor projects: %w", err)
		}

		rt.Detailf("Harbor rejected the admin password; enter it to create the missing projects")
		password = ""
	}

	return nil, nil, fmt.Errorf("could not authenticate as the Harbor admin to create the missing projects")
}

func harborAdminPasswordFromSecret(rt *core.Runtime) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	value, err := kube.ReadSecretKey(
		ctx,
		rt.Cluster.Client,
		rt.Bootstrap.Config.Harbor.Namespace,
		harborCoreSecret,
		harborAdminPasswordKey,
	)
	if err != nil {
		rt.Detailf("could not read the Harbor admin password from %s/%s: %v",
			rt.Bootstrap.Config.Harbor.Namespace, harborCoreSecret, err)
		return ""
	}
	return strings.TrimSpace(string(value))
}

func projectNames(client *harborapi.Client) (map[string]struct{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	projects, err := harborapi.ListProjects(ctx, client)
	if err != nil {
		return nil, err
	}

	names := make(map[string]struct{}, len(projects))
	for _, project := range projects {
		names[project.Name] = struct{}{}
	}
	return names, nil
}

func missingProjects(required []string, existing map[string]struct{}) []string {
	var missing []string
	for _, project := range required {
		if _, ok := existing[project]; !ok {
			missing = append(missing, project)
		}
	}
	return missing
}

// createPublicProject treats "already exists" as success: the project may
// have been created since it was listed.
func createPublicProject(client *harborapi.Client, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := harborapi.CreateProject(ctx, client, harborapi.CreateProjectRequest{
		Name:     name,
		Metadata: &harborapi.ProjectMetadata{Public: "true"},
	})

	var statusErr *httpapi.StatusError
	if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusConflict {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create Harbor project %q: %w", name, err)
	}
	return nil
}
