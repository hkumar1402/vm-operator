// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create;update,path=/default-mutate-vmoperator-vmware-com-v1alpha5-virtualmachinesnapshot,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinesnapshots,versions=v1alpha5,name=default.mutating.virtualmachinesnapshot.v1alpha5.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	hook, err := builder.NewMutatingWebhook(ctx, mgr, webHookName, NewMutator(mgr.GetClient()))
	if err != nil {
		return fmt.Errorf("failed to create mutation webhook: %w", err)
	}
	mgr.GetWebhookServer().Register(hook.Path, hook)

	return nil
}

// NewMutator returns the package's Mutator.
func NewMutator(client ctrlclient.Client) builder.Mutator {
	return mutator{
		client:    client,
		converter: runtime.DefaultUnstructuredConverter,
	}
}

type mutator struct {
	client    ctrlclient.Client
	converter runtime.UnstructuredConverter
}

func (m mutator) Mutate(ctx *pkgctx.WebhookRequestContext) admission.Response {
	if ctx.Op != admissionv1.Create {
		return admission.Allowed("")
	}

	modified, err := m.vmSnapshotFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	var wasMutated bool

	// Always set the VM name label on create
	if SetVMNameLabel(modified) {
		wasMutated = true
	}

	// Copy vCenter ID label from parent VM for multi-vCenter filtering
	if copied, err := m.copyVCenterLabelFromVM(ctx, modified); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	} else if copied {
		wasMutated = true
	}

	if !wasMutated {
		return admission.Allowed("")
	}

	rawModified, err := json.Marshal(modified)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(ctx.RawObj, rawModified)
}

func (m mutator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineSnapshot{}).Name())
}

// vmSnapshotFromUnstructured returns the VirtualMachineSnapshot from the unstructured object.
func (m mutator) vmSnapshotFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineSnapshot, error) {
	vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), vmSnapshot); err != nil {
		return nil, err
	}
	return vmSnapshot, nil
}

// SetVMNameLabel sets the VM name label on the snapshot if it has a vmRef.
// Returns true if the snapshot was mutated, false otherwise.
func SetVMNameLabel(vmSnapshot *vmopv1.VirtualMachineSnapshot) bool {
	// Only set the label if there's a vmName.
	if vmSnapshot.Spec.VMName == "" {
		return false
	}

	// Add the label if it does not exist.
	if _, exists := vmSnapshot.Labels[vmopv1.VMNameForSnapshotLabel]; !exists {
		vmName := vmSnapshot.Spec.VMName
		metav1.SetMetaDataLabel(&vmSnapshot.ObjectMeta, vmopv1.VMNameForSnapshotLabel, vmName)

		return true
	}

	return false
}

// copyVCenterLabelFromVM copies the vCenter ID label from the parent VM to the snapshot.
// This ensures per-vCenter containers only process snapshots for their VMs.
// Returns true if the label was copied, false if VM doesn't have the label or snapshot already has it.
func (m mutator) copyVCenterLabelFromVM(
	ctx *pkgctx.WebhookRequestContext,
	vmSnapshot *vmopv1.VirtualMachineSnapshot) (bool, error) {

	// Skip if snapshot already has vCenter label
	if vmSnapshot.Labels != nil && vmSnapshot.Labels[constants.VCenterIDLabel] != "" {
		return false, nil
	}

	// Skip if no VM name specified
	if vmSnapshot.Spec.VMName == "" {
		return false, nil
	}

	// Get the parent VM
	vm := &vmopv1.VirtualMachine{}
	vmKey := ctrlclient.ObjectKey{
		Name:      vmSnapshot.Spec.VMName,
		Namespace: vmSnapshot.Namespace,
	}
	if err := m.client.Get(ctx, vmKey, vm); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get parent VM %s: %w", vmKey, err)
	}

	// Copy vCenter label from VM to snapshot
	vmVCenterID := vm.Labels[constants.VCenterIDLabel]
	if vmVCenterID == "" {
		return false, nil
	}

	if vmSnapshot.Labels == nil {
		vmSnapshot.Labels = make(map[string]string)
	}
	vmSnapshot.Labels[constants.VCenterIDLabel] = vmVCenterID

	return true, nil
}
