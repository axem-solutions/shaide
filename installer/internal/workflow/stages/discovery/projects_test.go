package discovery

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/harbor/auth"
	"github.com/axem-solutions/ai_platform/installer/internal/logger"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	robotUser     = "robot$k8s-harbor-sa"
	robotPassword = "robot-secret"
	adminPassword = "admin-secret"
)

// fakeHarbor serves the two project endpoints the installer uses. The robot can
// list projects; only the admin can create them, as in Harbor.
type fakeHarbor struct {
	mu       sync.Mutex
	projects map[string]bool
	created  []map[string]any
}

func (h *fakeHarbor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	user, password, _ := r.BasicAuth()
	isAdmin := user == "admin" && password == adminPassword
	isRobot := user == robotUser && password == robotPassword

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v2.0/projects":
		if !isAdmin && !isRobot {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var list []map[string]any
		for name := range h.projects {
			list = append(list, map[string]any{"name": name})
		}
		_ = json.NewEncoder(w).Encode(list)

	case r.Method == http.MethodPost && r.URL.Path == "/api/v2.0/projects":
		if !isAdmin {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		name, _ := req["project_name"].(string)
		if h.projects[name] {
			http.Error(w, "conflict", http.StatusConflict)
			return
		}
		h.projects[name] = true
		h.created = append(h.created, req)
		w.WriteHeader(http.StatusCreated)

	default:
		http.NotFound(w, r)
	}
}

func (h *fakeHarbor) createdNames() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var names []string
	for _, req := range h.created {
		names = append(names, req["project_name"].(string))
	}
	sort.Strings(names)
	return names
}

type promptRecorder struct {
	answers []string
	asked   int
}

func (p *promptRecorder) Input(string, string, string) (string, error) {
	answer := ""
	if p.asked < len(p.answers) {
		answer = p.answers[p.asked]
	}
	p.asked++
	return answer, nil
}

func (*promptRecorder) Select(_, current string, _ []string) (string, error) { return current, nil }

func (*promptRecorder) MultiSelect(string, []string) ([]string, error) { return nil, nil }

func (*promptRecorder) ProgressModel(core.ModelProgress) {}

func harborCoreSecretWith(password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "harbor-core", Namespace: "harbor"},
		Data:       map[string][]byte{"HARBOR_ADMIN_PASSWORD": []byte(password)},
	}
}

func projectsRuntime(prompt *promptRecorder, objects ...runtime.Object) *core.Runtime {
	rt := core.NewContext(logger.NewWithWriter(io.Discard), prompt)
	rt.Cluster.Client = fake.NewSimpleClientset(objects...)
	rt.Bootstrap.Config.Harbor.Namespace = "harbor"
	rt.Bootstrap.Config.Harbor.Projects = []string{"ai-models", "shaide", "services"}
	rt.Bootstrap.Catalog.ServiceImages = []catalog.Image{
		{Project: "shaide", Name: "axem-solutions/shaide_server"},
		{Project: "services", Name: "busybox"},
	}
	rt.Bootstrap.Catalog.Models = []catalog.Model{
		{HarborProject: "ai-models", HarborName: "nomic-embed-text-v1.5"},
	}
	rt.Discovery.Auth = auth.Credentials{Username: robotUser, Password: robotPassword}
	return rt
}

func TestRequiredHarborProjectsCoverConfigAndCatalog(t *testing.T) {
	rt := projectsRuntime(&promptRecorder{})
	rt.Bootstrap.Catalog.ServiceImages = append(rt.Bootstrap.Catalog.ServiceImages,
		catalog.Image{Project: "extra", Name: "other"})

	got := RequiredHarborProjects(rt)
	want := []string{"ai-models", "extra", "services", "shaide"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RequiredHarborProjects() = %v, want %v", got, want)
	}
}

func TestHarborProjectForTarget(t *testing.T) {
	rt := projectsRuntime(&promptRecorder{})
	for target, want := range map[string]string{
		"axem-solutions/shaide_server": "shaide",
		"nomic-embed-text-v1.5":        "ai-models",
		"unknown":                      "",
	} {
		if got := HarborProjectForTarget(rt, target); got != want {
			t.Errorf("HarborProjectForTarget(%q) = %q, want %q", target, got, want)
		}
	}
}

// The common case: every project exists, so neither the admin password nor the
// operator is needed.
func TestEnsureHarborProjectsNothingMissing(t *testing.T) {
	harbor := &fakeHarbor{projects: map[string]bool{"ai-models": true, "shaide": true, "services": true}}
	server := httptest.NewServer(harbor)
	defer server.Close()
	prompt := &promptRecorder{}

	if err := ensureHarborProjectsAt(projectsRuntime(prompt), server.URL); err != nil {
		t.Fatalf("ensureHarborProjectsAt() error = %v", err)
	}
	if len(harbor.createdNames()) != 0 || prompt.asked != 0 {
		t.Errorf("created %v, prompted %d times; want nothing", harbor.createdNames(), prompt.asked)
	}
}

// The trial-westeurope case: a hand-installed Harbor without the installer's
// projects. They are created public with the admin password from harbor-core.
func TestEnsureHarborProjectsCreatesMissingAsPublic(t *testing.T) {
	harbor := &fakeHarbor{projects: map[string]bool{"ai-models": true, "images-shaide": true}}
	server := httptest.NewServer(harbor)
	defer server.Close()
	prompt := &promptRecorder{}

	if err := ensureHarborProjectsAt(projectsRuntime(prompt, harborCoreSecretWith(adminPassword)), server.URL); err != nil {
		t.Fatalf("ensureHarborProjectsAt() error = %v", err)
	}

	if got := harbor.createdNames(); !reflect.DeepEqual(got, []string{"services", "shaide"}) {
		t.Errorf("created %v, want [services shaide]", got)
	}
	for _, req := range harbor.created {
		metadata, _ := req["metadata"].(map[string]any)
		if metadata["public"] != "true" {
			t.Errorf("project %v created with metadata %v, want public=true", req["project_name"], metadata)
		}
		if _, ok := req["public"]; ok {
			t.Errorf("project %v sent the deprecated top-level public flag", req["project_name"])
		}
	}
	if prompt.asked != 0 {
		t.Errorf("prompted %d times; the password was in harbor-core", prompt.asked)
	}
}

// Without a readable harbor-core secret, or with a wrong password in it, the
// operator is asked, and asked again after a rejection.
func TestEnsureHarborProjectsPromptsForAdminPassword(t *testing.T) {
	harbor := &fakeHarbor{projects: map[string]bool{}}
	server := httptest.NewServer(harbor)
	defer server.Close()
	prompt := &promptRecorder{answers: []string{"wrong", adminPassword}}

	rt := projectsRuntime(prompt, harborCoreSecretWith("stale"))
	if err := ensureHarborProjectsAt(rt, server.URL); err != nil {
		t.Fatalf("ensureHarborProjectsAt() error = %v", err)
	}
	if prompt.asked != 2 {
		t.Errorf("prompted %d times, want 2 (the stale secret and the wrong answer are rejected)", prompt.asked)
	}
	if got := harbor.createdNames(); len(got) != 3 {
		t.Errorf("created %v, want all three projects", got)
	}
	if rt.Discovery.AdminPassword != adminPassword {
		t.Error("the accepted admin password is not kept for the rest of the run")
	}
}

// A project created between the listing and the create call is not an error.
func TestCreateExistingProjectIsNotAnError(t *testing.T) {
	harbor := &fakeHarbor{projects: map[string]bool{"shaide": true}}
	server := httptest.NewServer(harbor)
	defer server.Close()

	rt := projectsRuntime(&promptRecorder{}, harborCoreSecretWith(adminPassword))
	admin, _, err := harborAdminClient(rt, server.URL)
	if err != nil {
		t.Fatalf("harborAdminClient() error = %v", err)
	}
	if err := createPublicProject(admin, "shaide"); err != nil {
		t.Errorf("createPublicProject() on an existing project = %v, want nil", err)
	}
}

func TestHarborProjectExists(t *testing.T) {
	harbor := &fakeHarbor{projects: map[string]bool{"ai-models": true}}
	server := httptest.NewServer(harbor)
	defer server.Close()
	rt := projectsRuntime(&promptRecorder{})

	if exists, ok := harborProjectExistsAt(rt, server.URL, "shaide"); !ok || exists {
		t.Errorf("shaide: exists=%v ok=%v, want false,true", exists, ok)
	}
	if exists, ok := harborProjectExistsAt(rt, server.URL, "ai-models"); !ok || !exists {
		t.Errorf("ai-models: exists=%v ok=%v, want true,true", exists, ok)
	}

	rt.Discovery.Auth.Password = "rotated"
	if _, ok := harborProjectExistsAt(rt, server.URL, "shaide"); ok {
		t.Error("with rejected robot credentials the check must report it cannot tell")
	}
}
