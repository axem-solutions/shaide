package stacks

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	"k8s.io/client-go/dynamic"
)

func DeployGatewayProvider(rt *core.Runtime) error {
	platform := cluster.Provider(rt.Bootstrap.Provider)

	options, err := gatewayOptions(rt, platform)
	if err != nil {
		return err
	}

	gatewayStack := gateway.NewStack(
		filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectGatewayProvider),
		stack.Options{
			Platform:   platform,
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		options,
	)

	deployer, err := newStackDeployer(rt, gatewayStack, stackDeploymentOptions{})
	if err != nil {
		return err
	}

	// The Istio ownership sweep must tell this stack's objects from ones an
	// earlier Pulumi deployment wrote; only the stack's state can say which.
	resources, err := deployer.StateResources(context.Background())
	if err != nil {
		return fmt.Errorf("read gateway-provider stack state: %w", err)
	}
	gatewayStack.SetStateResources(resources)
	rt.Detailf("gateway-provider state holds %d resources", len(resources))

	// Take the hostname the operator supplied up front so later stages have it
	// even when the Gateway itself does not export one.
	if hostname, ok := gatewayStack.Hostname(deployer.ResolvedConfig()); ok {
		if hostname = strings.TrimSpace(hostname); hostname != "" {
			rt.Bootstrap.GatewayHostname = hostname
		}
	}

	result, err := deployer.Deploy(context.Background())
	if err != nil {
		return err
	}

	// A StackReference can supply the hostname asynchronously. Prefer the
	// resolved Pulumi output when present so downstream projects receive the
	// same hostname as the shared Gateway.
	if output, ok := result.Outputs["gatewayHostname"]; ok {
		if hostname, ok := output.Value.(string); ok && strings.TrimSpace(hostname) != "" {
			rt.Bootstrap.GatewayHostname = strings.TrimSpace(hostname)
		}
	}

	return nil
}

// gatewayOptions answers the gateway questions from the cluster: the class and
// the ClusterIssuer are picked from what exists, pre-selecting what the live
// Gateway uses, and the AGC load balancer is looked up only when AGC is the
// class. A question with a single possible answer is not asked.
func gatewayOptions(rt *core.Runtime, platform cluster.Provider) (gateway.Options, error) {
	// Each lookup has its own timeout: the prompts in between wait for the
	// operator, which a shared deadline would count against the lookups.
	ctx := context.Background()

	dyn, err := dynamic.NewForConfig(rt.Cluster.RESTConfig)
	if err != nil {
		return gateway.Options{}, fmt.Errorf("build dynamic client: %w", err)
	}

	live, err := readSharedGateway(ctx, dyn)
	if err != nil {
		rt.Detailf("could not read the existing shared Gateway (%v); nothing is pre-selected from it", err)
	}

	class, err := selectGatewayClass(ctx, rt, dyn, platform, live.class)
	if err != nil {
		return gateway.Options{}, err
	}

	var options gateway.Options
	if platform == cluster.Azure && class == gateway.AGCGatewayClassName {
		options = discoverALB(ctx, dyn, rt.Cluster.Client, rt.Detailf)
	}
	options.GatewayClassName = class

	options.CertManagerIssuer, err = selectCertManagerIssuer(ctx, rt, dyn, live.issuer)
	if err != nil {
		return gateway.Options{}, err
	}

	return options, nil
}
