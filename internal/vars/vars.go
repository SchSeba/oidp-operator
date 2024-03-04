package vars

import "os"

var (
	// ArgoNamespace represent the namespace where argo is deployed
	ArgoNamespace = "argo"
)

func init() {
	argoNamespace := os.Getenv("")
	if argoNamespace != "" {
		ArgoNamespace = argoNamespace
	}
}
