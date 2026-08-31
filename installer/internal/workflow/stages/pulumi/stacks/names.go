package stacks

const (
	projectGatewayProvider = "gateway-provider"
	projectAppServing      = "app-serving"
	projectAppShaide       = "app-shaide"
	projectHarbor          = "harbor"
	projectOnPremHarbor    = "k8s-onprem-airgap-services"
	projectMonitoring      = "monitoring"

	stackGatewayProvider = "provider"
	stackAppServing      = "serving"
	stackAppShaide       = "shaide"
	stackHarbor          = "harbor"
	stackOnPremHarbor    = "OnPrem"
	stackMonitoring      = "monitoring"

	configNamespaceHarbor = "harbor"
)

func pulumiConfigKey(projectName, key string) string {
	return projectName + ":" + key
}
