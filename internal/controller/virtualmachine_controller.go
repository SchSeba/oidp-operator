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
	"context"
	"fmt"
	"github.com/SchSeba/oidp-operator/internal/consts"
	"github.com/SchSeba/oidp-operator/internal/vars"
	argo "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
	logger.WithName("Reconcile")

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
		// check if we need to add a finalizer to the object
		updated := controllerutil.AddFinalizer(instance, consts.FinalizerName)
		if updated {
			err = r.Update(ctx, instance)
			if err != nil {
				logger.Error(err, "failed to add finalizer to virtual machine instance", "virtualMachine", *instance)
			}
			return ctrl.Result{}, err
		}

		// Check if the workload exist
		// if the workflow doesn't exist we create a new one
		if instance.Status.WorkflowName == "" {
			// there is no workflow create a new creation workflow
			err = r.CreateWorkflow(ctx, instance)
			return ctrl.Result{}, err
		}
	} else {
		// we need to run the workflow to remove the object
		err = r.DeleteWorkflow(ctx, instance)
		if err != nil {
			logger.Error(err, "failed to create the delete argo workflow", "instance", *instance)
			return ctrl.Result{}, err
		}
	}

	// let's check the state of the workflow
	workflow := &argo.Workflow{}
	err = r.Get(ctx, client.ObjectKey{Name: instance.Status.WorkflowName, Namespace: vars.ArgoNamespace}, workflow)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("the argo workflow doesn't exist", "workflowName", instance.Status.WorkflowName, "instance", *instance)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get the argo workflow", "workflowName", instance.Status.WorkflowName, "instance", *instance)
		return ctrl.Result{}, err
	}

	// Update instance conditions
	err = r.UpdateConditions(ctx, instance, workflow)
	return ctrl.Result{}, err
}

func (r *VirtualMachineReconciler) CreateWorkflow(ctx context.Context, instance *platformv1alpha1.VirtualMachine) error {
	logger := log.FromContext(ctx)
	logger.WithName("CreateWorkflow")

	// create the data dict to pass into the template
	data := map[string]interface{}{
		"ipAddr":    instance.Spec.IpAddress,
		"remove":    false,
		"namespace": vars.ArgoNamespace,
	}

	// create the workflow template
	workflow, err := resources.CreateWorkflowFromTemplate(resources.VmWorkflow, data)
	if err != nil {
		logger.Error(err, "failed to create vm workflow from template", "data", data)
		return err
	}

	if workflow.Labels == nil {
		workflow.Labels = map[string]string{}
	}

	// add the information needed for as to find the object as a callback for the reconcile loop
	workflow.Labels[consts.ObjectName] = instance.Name
	workflow.Labels[consts.ObjectNamespace] = instance.Namespace
	workflow.Labels[consts.ObjectType] = consts.ObjectTypeVM

	// apply the workflow
	err = r.Create(ctx, workflow)
	if err != nil {
		return err
	}

	// Save the workflow name to the instance status
	instance.Status.WorkflowName = workflow.Name
	err = r.Status().Update(ctx, instance)
	if err != nil {
		logger.Error(err, "failed to update the vm instance with the workflow name")
	}
	return err
}

func (r *VirtualMachineReconciler) UpdateConditions(ctx context.Context, instance *platformv1alpha1.VirtualMachine, workflow *argo.Workflow) error {
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = []metav1.Condition{}
	}

	switch workflow.Status.Phase {
	case argo.WorkflowRunning:
		if meta.IsStatusConditionTrue(instance.Status.Conditions, string(platformv1alpha1.PlatformRunning)) {
			return nil
		}

		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Status:             metav1.ConditionTrue,
			Type:               string(platformv1alpha1.PlatformRunning),
			Message:            fmt.Sprintf("workflow: %s", workflow.Name),
			Reason:             "workflowRunning",
			LastTransitionTime: metav1.Now(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Status: metav1.ConditionFalse,
			Type:   string(platformv1alpha1.PlatformFailed),
			Reason: "workflowFailed",
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Status: metav1.ConditionFalse,
			Type:   string(platformv1alpha1.PlatformCreated),
			Reason: "workflowCreated",
		})
	case argo.WorkflowSucceeded:
		if meta.IsStatusConditionTrue(instance.Status.Conditions, string(platformv1alpha1.PlatformCreated)) {
			return nil
		}

		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Status: metav1.ConditionFalse,
			Type:   string(platformv1alpha1.PlatformRunning),
			Reason: "workflowRunning",
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Status: metav1.ConditionFalse,
			Type:   string(platformv1alpha1.PlatformFailed),
			Reason: "workflowFailed",
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Status:             metav1.ConditionTrue,
			Type:               string(platformv1alpha1.PlatformCreated),
			Message:            fmt.Sprintf("workflow: %s", workflow.Name),
			Reason:             "workflowCreated",
			LastTransitionTime: metav1.Now(),
		})
	case argo.WorkflowError, argo.WorkflowFailed:
		if meta.IsStatusConditionTrue(instance.Status.Conditions, string(platformv1alpha1.PlatformFailed)) {
			return nil
		}

		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Status: metav1.ConditionFalse,
			Type:   string(platformv1alpha1.PlatformRunning),
			Reason: "workflowRunning",
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Status:             metav1.ConditionTrue,
			Type:               string(platformv1alpha1.PlatformFailed),
			Message:            fmt.Sprintf("workflow: %s", workflow.Name),
			LastTransitionTime: metav1.Now(),
			Reason:             "workflowFailed",
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Status: metav1.ConditionFalse,
			Type:   string(platformv1alpha1.PlatformCreated),
			Reason: "workflowCreated",
		})
	}

	return r.Status().Update(ctx, instance)
}

func (r *VirtualMachineReconciler) DeleteWorkflow(ctx context.Context, instance *platformv1alpha1.VirtualMachine) error {
	logger := log.FromContext(ctx)
	logger.WithName("DeleteWorkflow")

	// we start the remove lets check if the workflow is done
	if instance.Status.Removing {
		workflow := &argo.Workflow{}
		err := r.Get(ctx, client.ObjectKey{Name: instance.Status.WorkflowName, Namespace: vars.ArgoNamespace}, workflow)
		if err != nil {
			// if the object doesn't exist we may don't have it in the cache lets just leave
			// and let the reconcile loop to get called again
			if errors.IsNotFound(err) {
				return nil
			}

			// if there is a different error lets requeue
			return err
		}

		// we find the object lets check the status
		// if the workflow is done lets release the finalizer
		if workflow.Status.Phase == argo.WorkflowSucceeded {
			controllerutil.RemoveFinalizer(instance, "oidp-delete")
			err = r.Update(ctx, instance)
		}

		return err
	}

	// let's mark the object os removing and create the remove workflow
	instance.Status.Removing = true

	// create the data dict to pass into the template
	data := map[string]interface{}{
		"remove":    true,
		"namespace": vars.ArgoNamespace,
	}

	// create the workflow template
	workflow, err := resources.CreateWorkflowFromTemplate(resources.VmWorkflow, data)
	if err != nil {
		logger.Error(err, "failed to create vm workflow from template", "data", data)
		return err
	}

	if workflow.Labels == nil {
		workflow.Labels = map[string]string{}
	}

	// add the information needed for as to find the object as a callback for the reconcile loop
	workflow.Labels[consts.ObjectName] = instance.Name
	workflow.Labels[consts.ObjectNamespace] = instance.Namespace
	workflow.Labels[consts.ObjectType] = consts.ObjectTypeVM

	// apply the workflow
	err = r.Create(ctx, workflow)
	if err != nil {
		return err
	}

	// Save the workflow name to the instance status
	instance.Status.WorkflowName = workflow.Name
	err = r.Status().Update(ctx, instance)
	if err != nil {
		logger.Error(err, "failed to update the vm instance with the workflow name")
	}
	return err
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

			return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}}
		}

		return nil
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.VirtualMachine{}).
		Watches(&argo.Workflow{}, handler.EnqueueRequestsFromMapFunc(findVmFunc)).
		Complete(r)
}
