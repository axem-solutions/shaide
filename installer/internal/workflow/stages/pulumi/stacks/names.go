package stacks

const (
	projectGatewayProvider = "gateway-provider"
	projectAppServing      = "app-serving"
	projectAppShaide       = "app-shaide"
	projectCloudHarbor     = "cloud-harbor"
	projectOnPremHarbor    = "k8s-onprem-airgap-services"
	projectMonitoring      = "monitoring"

	stackGatewayProvider = "provider"
	stackAppServing      = "serving"
	stackAppShaide       = "shaide"
	stackCloudHarbor     = "harbor"
	stackOnPremHarbor    = "OnPrem"
)

func pulumiConfigKey(projectName, key string) string {
	return projectName + ":" + key
}
