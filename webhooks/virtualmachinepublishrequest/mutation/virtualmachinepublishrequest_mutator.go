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

// +kubebuilder:webhook:verbs=create,path=/default-mutate-vmoperator-vmware-com-v1alpha5-virtualmachinepublishrequest,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinepublishrequests,versions=v1alpha5,name=default.mutating.virtualmachinepublishrequest.v1alpha5.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=virtualmachines,verbs=get;list

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

	modified, err := m.publishRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Copy vCenter ID label from source VM
	if copied, err := m.copyVCenterLabelFromSourceVM(ctx, modified); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	} else if !copied {
		return admission.Allowed("")
	}

	rawModified, err := json.Marshal(modified)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(ctx.RawObj, rawModified)
}

func (m mutator) For() schema.GroupVersionKind {
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachinePublishRequest{}).Name())
}

// publishRequestFromUnstructured returns the VirtualMachinePublishRequest from the unstructured object.
func (m mutator) publishRequestFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachinePublishRequest, error) {
	publishRequest := &vmopv1.VirtualMachinePublishRequest{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), publishRequest); err != nil {
		return nil, err
	}
	return publishRequest, nil
}

// copyVCenterLabelFromSourceVM copies the vCenter ID label from the source VM to the publish request.
// This ensures per-vCenter containers only process publish requests for their VMs.
// Returns true if the label was copied, false if VM doesn't have the label or request already has it.
func (m mutator) copyVCenterLabelFromSourceVM(
	ctx *pkgctx.WebhookRequestContext,
	publishRequest *vmopv1.VirtualMachinePublishRequest) (bool, error) {

	// Skip if request already has vCenter label
	if publishRequest.Labels != nil && publishRequest.Labels[constants.VCenterIDLabel] != "" {
		return false, nil
	}

	// Determine source VM name
	var vmName string
	if publishRequest.Spec.Source.Name != "" {
		vmName = publishRequest.Spec.Source.Name
	} else {
		// If source name is omitted, controller uses the publish request's name
		vmName = publishRequest.Name
	}

	if vmName == "" {
		return false, nil
	}

	// Get the source VM
	vm := &vmopv1.VirtualMachine{}
	vmKey := ctrlclient.ObjectKey{
		Name:      vmName,
		Namespace: publishRequest.Namespace,
	}
	if err := m.client.Get(ctx, vmKey, vm); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get source VM %s: %w", vmKey, err)
	}

	// Copy vCenter label from VM to publish request
	vmVCenterID := vm.Labels[constants.VCenterIDLabel]
	if vmVCenterID == "" {
		return false, nil
	}

	if publishRequest.Labels == nil {
		publishRequest.Labels = make(map[string]string)
	}
	publishRequest.Labels[constants.VCenterIDLabel] = vmVCenterID

	return true, nil
}
