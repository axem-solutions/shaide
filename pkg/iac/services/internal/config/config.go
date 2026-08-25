package config

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// StackConfig provides typed accessors for all stack configuration values.
// Keys follow the Pulumi convention: <namespace>:<key>.
// Secrets are returned as pulumi.StringOutput so they stay encrypted in state.
type StackConfig struct {
	base        *pulumiconfig.Config
	harbor      *pulumiconfig.Config
	metallb     *pulumiconfig.Config
	gpuOperator *pulumiconfig.Config
}

func NewStackConfig(ctx *pulumi.Context) *StackConfig {
	return &StackConfig{
		base:        pulumiconfig.New(ctx, ""),
		harbor:      pulumiconfig.New(ctx, "harbor"),
		metallb:     pulumiconfig.New(ctx, "metallb"),
		gpuOperator: pulumiconfig.New(ctx, "gpu-operator"),
	}
}

// Kubeconfig returns the path to the kubeconfig file for the target cluster.
func (c *StackConfig) Kubeconfig() string {
	return c.base.Require("kubeconfig")
}

// Components returns the set of component names enabled for this stack.
// Defined in Pulumi.<stack>.yaml under the key "components".
func (c *StackConfig) Components() map[string]bool {
	var list []string
	c.base.RequireObject("components", &list)
	set := make(map[string]bool, len(list))
	for _, name := range list {
		set[name] = true
	}
	return set
}

// ── Harbor ────────────────────────────────────────────────────────────────────

// HarborAdminPassword returns the Harbor admin password as a secret output.
func (c *StackConfig) HarborAdminPassword() pulumi.StringOutput {
	return c.harbor.RequireSecret("adminPassword")
}

// HarborRobotPassword returns the Harbor robot account password and whether it
// is configured. The password is defined in ansible vault (vault_harbor_robot_password)
// and must be registered with Pulumi after ansible/harbor_setup.yml runs:
//
//	pulumi config set --secret harbor:robotPassword $(cat artifacts/harbor-robot-secret)
//
// On the first pulumi up (Harbor bootstrap), the value is not yet set and the
// pull secret creation is skipped. Re-run pulumi up after registering the password.
func (c *StackConfig) HarborRobotPassword() (pulumi.StringOutput, bool) {
	if v := c.harbor.Get("robotPassword"); v == "" {
		return pulumi.String("").ToStringOutput(), false
	}
	return c.harbor.RequireSecret("robotPassword"), true
}

// HarborHostname returns the internal hostname for Harbor.
// Must match harbor_hostname in ansible/group_vars/all/harbor.yml.
func (c *StackConfig) HarborHostname() string {
	return c.harbor.Require("hostname")
}

// HarborNodeHostname returns the Kubernetes node name where Harbor pods run.
// Must match harbor_node_hostname in harbor.yml.
func (c *StackConfig) HarborNodeHostname() string {
	return c.harbor.Require("nodeHostname")
}

// HarborProjectName returns the Harbor project where infrastructure images are stored.
func (c *StackConfig) HarborProjectName() string {
	return c.harbor.Require("projectName")
}

// HarborChartPath returns the local path to the Harbor Helm chart tarball.
func (c *StackConfig) HarborChartPath() string {
	if v := c.harbor.Get("chartPath"); v != "" {
		return v
	}
	return "./charts/harbor-1.18.2.tgz"
}

// ── MetalLB ───────────────────────────────────────────────────────────────────

// MetalLBIPPool returns the MetalLB IP address pool range (e.g. "10.99.10.200-10.99.10.220").
func (c *StackConfig) MetalLBIPPool() string {
	return c.metallb.Require("ipPool")
}

// MetalLBControllerNodeHostname returns the node where the MetalLB controller pod runs.
func (c *StackConfig) MetalLBControllerNodeHostname() string {
	return c.metallb.Require("controllerNodeHostname")
}

// MetalLBL2Interface returns the NIC name to restrict L2 announcements to.
// Returns empty string if not configured (MetalLB will use any interface).
func (c *StackConfig) MetalLBL2Interface() string {
	return c.metallb.Get("l2Interface")
}

// MetalLBChartPath returns the local path to the MetalLB Helm chart tarball.
func (c *StackConfig) MetalLBChartPath() string {
	if v := c.metallb.Get("chartPath"); v != "" {
		return v
	}
	return "./charts/metallb-0.15.3.tgz"
}

// ── GPU Operator ──────────────────────────────────────────────────────────────

// GPUOperatorGPUNodeHostname returns the node where the GPU Operator controller
// pod runs (Worker Node 2 — the GPU node).
func (c *StackConfig) GPUOperatorGPUNodeHostname() string {
	return c.gpuOperator.Require("gpuNodeHostname")
}

// GPUOperatorChartPath returns the local path to the GPU Operator Helm chart tarball.
func (c *StackConfig) GPUOperatorChartPath() string {
	if v := c.gpuOperator.Get("chartPath"); v != "" {
		return v
	}
	return "./charts/gpu-operator-v25.10.1.tgz"
}
