package resources

import (
	_ "embed"
)

var (
	//go:embed files/vm.yaml
	VmWorkflow string
)
