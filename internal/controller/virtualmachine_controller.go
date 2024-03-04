/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"bytes"
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"text/template"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	argo "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"

	platformv1alpha1 "github.com/SchSeba/oidp-operator/api/v1alpha1"
	"github.com/SchSeba/oidp-operator/internal/resources"
)

// VirtualMachineReconciler reconciles a VirtualMachine object
type VirtualMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=platform.example.com,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=platform.example.com,resources=virtualmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=platform.example.com,resources=virtualmachines/finalizers,verbs=update
//+kubebuilder:rbac:groups=argoproj.io,resources=Workflow,verbs=get;list;watch;create;update;patch;delete

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *VirtualMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	instance := &platformv1alpha1.VirtualMachine{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "failed to find VirtualMachine object",
			"VirtualMachineName",
			req.Name,
			"Namespace",
			req.Namespace)
		return reconcile.Result{}, err
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		exist := false
		for _, fin := range instance.Finalizers {
			if fin == "oidp-delete" {
				exist = true
				break
			}
		}

		if !exist {
			instance.Finalizers = append(instance.Finalizers, "oidp-delete")
			if err := r.Update(ctx, instance); err != nil {
				return reconcile.Result{}, err
			}
		}

		// Check if the workload exist and is running
		workflow := &argo.Workflow{}
		err = r.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("vm-%s-%d", instance.Name, instance.Generation), Namespace: "argo"}, workflow)
		if err != nil {
			if errors.IsNotFound(err) {
				// New/Update object
				err = r.CreateVmWorkflow(ctx, instance)
				if err != nil {
					logger.Error(err, "failed to create vm object",
						"VirtualMachineName",
						req.Name,
						"Namespace",
						req.Namespace)
					return ctrl.Result{}, err
				}
			}
		}
	} else {
		// the object we requested to be deleted
		err = r.DeleteVmWorkflow(ctx, instance)
		if err != nil {
			logger.Error(err, "failed to remove vm object",
				"VirtualMachineName",
				req.Name,
				"Namespace",
				req.Namespace)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *VirtualMachineReconciler) CreateVmWorkflow(ctx context.Context, instance *platformv1alpha1.VirtualMachine) error {
	workflow, err := loadAndUnmarshalWorkflowTemplate(instance)
	if err != nil {
		return err
	}

	err = r.Create(ctx, workflow)
	if err != nil {
		return err
	}

	return nil
}

func (r *VirtualMachineReconciler) DeleteVmWorkflow(ctx context.Context, instance *platformv1alpha1.VirtualMachine) error {
	instance.Status.Removing = true

	workflow, err := loadAndUnmarshalWorkflowTemplate(instance)
	if err != nil {
		return err
	}

	err = r.Create(ctx, workflow)
	if err != nil {
		return err
	}

	err = r.Status().Update(ctx, instance)
	if err != nil {
		return err
	}

	return nil
}

func loadAndUnmarshalWorkflowTemplate(instance *platformv1alpha1.VirtualMachine) (*argo.Workflow, error) {
	tmpl, err := template.New("vm").Parse(resources.VmWorkflow)
	if err != nil {
		return nil, err
	}
	var tpl bytes.Buffer
	err = tmpl.Execute(&tpl, *instance)
	if err != nil {
		return nil, err
	}

	workflow := &argo.Workflow{}
	err = yaml.Unmarshal(tpl.Bytes(), workflow)
	if err != nil {
		return nil, err
	}

	if workflow.Labels == nil {
		workflow.Labels = map[string]string{}
	}
	workflow.Labels["oidp-object-name"] = fmt.Sprintf(instance.Name)
	workflow.Labels["oidp-object-namespace"] = fmt.Sprintf(instance.Namespace)
	workflow.Labels["oidp-object-type"] = "vm"

	// change namespace to the user namespace
	workflow.Namespace = "argo" //TODO: change this and use a variable

	return workflow, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtualMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	findVmFunc := func(ctx context.Context, object client.Object) []reconcile.Request {
		if value, exist := object.GetLabels()["oidp-object-type"]; exist && value == "vm" {
			name, exist := object.GetLabels()["oidp-object-name"]
			if !exist {
				return nil
			}
			namespace, exist := object.GetLabels()["oidp-object-namespace"]
			if !exist {
				return nil
			}

			return []reconcile.Request{{types.NamespacedName{Name: name, Namespace: namespace}}}
		}

		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.VirtualMachine{}).
		Watches(&argo.Workflow{}, handler.EnqueueRequestsFromMapFunc(findVmFunc)).
		Complete(r)
}
