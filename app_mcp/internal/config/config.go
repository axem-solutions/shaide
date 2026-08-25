package appconfig

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// Datasource describes a single MCP Server deployment for one data source.
type Datasource struct {
	Name          string   `json:"name"`          // used in resource names: mcp-<name>
	Image         string   `json:"image"`         // fully qualified container image
	Port          int      `json:"port"`          // MCP server listen port; default 8080
	Args          []string `json:"args"`          // command-line args passed to the MCP container
	CACert        string   `json:"caCert"`        // PEM-encoded CA certificate for this datasource's TLS; omit if not needed
	CATrustEnvVar string   `json:"caTrustEnvVar"` // NODE_EXTRA_CA_CERTS | REQUESTS_CA_BUNDLE | SSL_CERT_FILE
	EgressHost    string   `json:"egressHost"`    // datasource hostname (e.g. "jira.company.internal"); stored as annotation, cannot be used in NetworkPolicy ipBlock
	EgressCIDR    string   `json:"egressCIDR"`    // datasource IP/CIDR (e.g. "10.20.5.10/32"); required for egress NetworkPolicy; omit to skip policy
	EgressPort    int      `json:"egressPort"`    // TCP port to the datasource; default 443

	Env       map[string]string `json:"env"`       // arbitrary env vars injected into the MCP container at runtime
	SecretEnv []SecretEnvVar    `json:"secretEnv"` // env vars sourced from Kubernetes Secrets in the MCP namespace

	// Per-datasource overrides — each falls back to the corresponding global Config value when empty/zero.
	Replicas                     int    `json:"replicas"`                     // pod replica count; 0 = use global default (1)
	ImagePullPolicy              string `json:"imagePullPolicy"`              // Always | IfNotPresent | Never; empty = use global default
	HealthPath                   string `json:"healthPath"`                   // HTTP path for startup/readiness/liveness probes; empty = use global default
	DisableProbes                bool   `json:"disableProbes"`                // disables startup/readiness/liveness probes for images without health endpoints
	CPURequest                   string `json:"cpuRequest"`                   // e.g. "200m"; empty = use global default
	MemoryRequest                string `json:"memoryRequest"`                // e.g. "256Mi"; empty = use global default
	CPULimit                     string `json:"cpuLimit"`                     // e.g. "1000m"; empty = use global default
	MemoryLimit                  string `json:"memoryLimit"`                  // e.g. "1Gi"; empty = use global default
	StartupProbeFailureThreshold int    `json:"startupProbeFailureThreshold"` // 0 = use global default; controls max startup window (threshold × period)
	NodeSelector                 string `json:"nodeSelector"`                 // optional node selector value; empty = use global default
}

type SecretEnvVar struct {
	Name       string `json:"name"`       // env var name in the container
	SecretName string `json:"secretName"` // Kubernetes Secret name in cfg.Namespace
	SecretKey  string `json:"secretKey"`  // key inside the Kubernetes Secret
}

// Config is the typed view of Pulumi stack config used by this stack.
type Config struct {
	Namespace                string   // mcp namespace
	ShaideNamespace          string   // app-shaide namespace — where the Shaide Server ServiceAccount lives
	ShaideServiceAccountName string   // Shaide Server ServiceAccount name in ShaideNamespace; must match app_shaide stack config
	Kubeconfig               string   // optional — path to kubeconfig file; empty = use KUBECONFIG env / ~/.kube/config
	CompanyCACert            string   // PEM-encoded company internal root CA cert; per-datasource CACert takes precedence
	CompanyCATrustEnvVar     string   // fallback trust env var when datasource.CATrustEnvVar is empty; NODE_EXTRA_CA_CERTS | REQUESTS_CA_BUNDLE | SSL_CERT_FILE
	NodeSelectorKey          string   // label key used for node selection (default: nodegroup)
	NodeSelector             string   // global node selector value applied to all MCP datasources when set
	ImagePullSecrets         []string // optional imagePullSecret names in the MCP namespace

	// Deployment defaults — each can be overridden per datasource.
	ImagePullPolicy string // Always | IfNotPresent | Never; default: Always
	HealthPath      string // HTTP path for startup/readiness/liveness probes; default: /health
	CPURequest      string // default: 100m
	MemoryRequest   string // default: 128Mi
	CPULimit        string // default: 500m
	MemoryLimit     string // default: 512Mi

	// Probe timing — global only (not overridable per datasource).
	StartupProbePeriod             int // seconds between startup probe checks; default 5
	StartupProbeTimeout            int // probe timeout in seconds; default 3
	StartupProbeFailureThreshold   int // max failures before pod is killed; default 30 (= 150s window)
	ReadinessProbeInitialDelay     int // seconds before first readiness check; default 5
	ReadinessProbePeriod           int // seconds between readiness checks; default 10
	ReadinessProbeTimeout          int // probe timeout in seconds; default 3
	ReadinessProbeFailureThreshold int // consecutive failures before pod is marked unready; default 3
	LivenessProbeInitialDelay      int // seconds before first liveness check; default 15
	LivenessProbePeriod            int // seconds between liveness checks; default 30
	LivenessProbeTimeout           int // probe timeout in seconds; default 5
	LivenessProbeFailureThreshold  int // consecutive failures before pod is restarted; default 3

	Datasources []Datasource // one entry per MCP data source to deploy

	Secrets AppSecrets
}

type AppSecrets struct {
	AtlassianOAuthClientSecret    pulumi.StringOutput
	HasAtlassianOAuthClientSecret bool
}

func getWithDefault(cfg *pulumiconfig.Config, key, fallback string) string {
	v := cfg.Get(key)
	if v == "" {
		return fallback
	}
	return v
}

func getIntOrDefault(cfg *pulumiconfig.Config, key string, fallback int) int {
	v := cfg.GetInt(key)
	if v == 0 {
		return fallback
	}
	return v
}

func Load(ctx *pulumi.Context) Config {
	cfg := pulumiconfig.New(ctx, "")

	var datasources []Datasource
	cfg.RequireObject("datasources", &datasources)

	var imagePullSecrets []string
	cfg.GetObject("imagePullSecrets", &imagePullSecrets)

	hasAtlassianOAuthClientSecret := cfg.Get("mcpAtlassianOAuthClientSecret") != ""
	atlassianOAuthClientSecret := pulumi.String("").ToStringOutput()
	if hasAtlassianOAuthClientSecret {
		atlassianOAuthClientSecret = cfg.RequireSecret("mcpAtlassianOAuthClientSecret")
		addAtlassianSecretEnv(datasources)
	}

	return Config{
		Namespace:                cfg.Require("namespace"),
		ShaideNamespace:          cfg.Require("shaideNamespace"),
		ShaideServiceAccountName: cfg.Require("shaideServiceAccountName"),
		Kubeconfig:               cfg.Get("kubeconfig"),
		CompanyCACert:            cfg.Get("companyCACert"),
		CompanyCATrustEnvVar:     cfg.Get("companyCATrustEnvVar"),
		NodeSelectorKey:          getWithDefault(cfg, "nodeSelectorKey", "nodegroup"),
		NodeSelector:             cfg.Get("nodeSelector"),
		ImagePullSecrets:         imagePullSecrets,

		ImagePullPolicy: getWithDefault(cfg, "imagePullPolicy", "Always"),
		HealthPath:      getWithDefault(cfg, "healthPath", "/health"),
		CPURequest:      getWithDefault(cfg, "cpuRequest", "100m"),
		MemoryRequest:   getWithDefault(cfg, "memoryRequest", "128Mi"),
		CPULimit:        getWithDefault(cfg, "cpuLimit", "500m"),
		MemoryLimit:     getWithDefault(cfg, "memoryLimit", "512Mi"),

		StartupProbePeriod:             getIntOrDefault(cfg, "startupProbePeriod", 5),
		StartupProbeTimeout:            getIntOrDefault(cfg, "startupProbeTimeout", 3),
		StartupProbeFailureThreshold:   getIntOrDefault(cfg, "startupProbeFailureThreshold", 30),
		ReadinessProbeInitialDelay:     getIntOrDefault(cfg, "readinessProbeInitialDelay", 5),
		ReadinessProbePeriod:           getIntOrDefault(cfg, "readinessProbePeriod", 10),
		ReadinessProbeTimeout:          getIntOrDefault(cfg, "readinessProbeTimeout", 3),
		ReadinessProbeFailureThreshold: getIntOrDefault(cfg, "readinessProbeFailureThreshold", 3),
		LivenessProbeInitialDelay:      getIntOrDefault(cfg, "livenessProbeInitialDelay", 15),
		LivenessProbePeriod:            getIntOrDefault(cfg, "livenessProbePeriod", 30),
		LivenessProbeTimeout:           getIntOrDefault(cfg, "livenessProbeTimeout", 5),
		LivenessProbeFailureThreshold:  getIntOrDefault(cfg, "livenessProbeFailureThreshold", 3),

		Datasources: datasources,
		Secrets: AppSecrets{
			AtlassianOAuthClientSecret:    atlassianOAuthClientSecret,
			HasAtlassianOAuthClientSecret: hasAtlassianOAuthClientSecret,
		},
	}
}

func addAtlassianSecretEnv(datasources []Datasource) {
	for i := range datasources {
		if datasources[i].Name != "atlassian" || hasSecretEnv(datasources[i].SecretEnv, "ATLASSIAN_OAUTH_CLIENT_SECRET") {
			continue
		}
		datasources[i].SecretEnv = append(datasources[i].SecretEnv, SecretEnvVar{
			Name:       "ATLASSIAN_OAUTH_CLIENT_SECRET",
			SecretName: "mcp-secrets",
			SecretKey:  "ATLASSIAN_OAUTH_CLIENT_SECRET",
		})
	}
}

func hasSecretEnv(secretEnv []SecretEnvVar, name string) bool {
	for _, ref := range secretEnv {
		if ref.Name == name {
			return true
		}
	}
	return false
}
