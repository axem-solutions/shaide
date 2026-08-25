// App Shaide application stack orchestrator.
//
// Creates all shared dependencies and dispatches each component to its own file:
//   - deploy-namespace.go          (Namespace)
//   - deploy-app-shaide-config.go  (ConfigMap, Secret)
//   - ghcr-secret.go               (GitHub Container Registry pull secret)
//   - deploy-shaide-server.go      (StatefulSet, LoadBalancer)
//   - deploy-control-panel.go      (Deployment, ClusterIP)
//   - deploy-webapp.go             (Deployment, ClusterIP)
//   - deploy-rustfs.go             (StatefulSet, ClusterIP)
//   - deploy-qdrant.go             (StatefulSet, ClusterIP)
package main

import (
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		return shaide.DeployAppShaide(ctx)
	})
}
