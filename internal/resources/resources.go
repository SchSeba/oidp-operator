package resources

import (
	"bytes"
	_ "embed"
	"text/template"

	"k8s.io/apimachinery/pkg/util/yaml"

	argo "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
)

var (
	//go:embed files/vm.yaml
	VmWorkflow string
)

func CreateWorkflowFromTemplate(workflowStr string, values map[string]interface{}) (*argo.Workflow, error) {
	tmpl, err := template.New("workflow").Parse(workflowStr)
	if err != nil {
		return nil, err
	}
	var tpl bytes.Buffer
	err = tmpl.Execute(&tpl, values)
	if err != nil {
		return nil, err
	}

	workflow := &argo.Workflow{}
	err = yaml.Unmarshal(tpl.Bytes(), workflow)
	if err != nil {
		return nil, err
	}

	return workflow, nil
}
