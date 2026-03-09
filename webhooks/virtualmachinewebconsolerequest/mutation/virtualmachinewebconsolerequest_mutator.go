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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	webhookcommon "github.com/vmware-tanzu/vm-operator/webhooks/common"
)

const (
	webHookName = "default"
)

// +kubebuilder:webhook:verbs=create,path=/default-mutate-vmoperator-vmware-com-v1alpha5-virtualmachinewebconsolerequest,mutating=true,failurePolicy=fail,groups=vmoperator.vmware.com,resources=virtualmachinewebconsolerequests,versions=v1alpha5,name=default.mutating.virtualmachinewebconsolerequest.v1alpha5.vmoperator.vmware.com,sideEffects=None,admissionReviewVersions=v1;v1beta1
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

	modified, err := m.webConsoleRequestFromUnstructured(ctx.Obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Copy vCenter ID label from parent VM
	if copied, err := m.copyVCenterLabelFromVM(ctx, modified); err != nil {
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
	return vmopv1.GroupVersion.WithKind(reflect.TypeOf(vmopv1.VirtualMachineWebConsoleRequest{}).Name())
}

// webConsoleRequestFromUnstructured returns the VirtualMachineWebConsoleRequest from the unstructured object.
func (m mutator) webConsoleRequestFromUnstructured(obj runtime.Unstructured) (*vmopv1.VirtualMachineWebConsoleRequest, error) {
	webConsoleRequest := &vmopv1.VirtualMachineWebConsoleRequest{}
	if err := m.converter.FromUnstructured(obj.UnstructuredContent(), webConsoleRequest); err != nil {
		return nil, err
	}
	return webConsoleRequest, nil
}

// copyVCenterLabelFromVM copies the vCenter ID label from the referenced VM to the web console request.
// This ensures per-vCenter containers only process requests for their VMs.
func (m mutator) copyVCenterLabelFromVM(
	ctx *pkgctx.WebhookRequestContext,
	webConsoleRequest *vmopv1.VirtualMachineWebConsoleRequest) (bool, error) {

	return webhookcommon.CopyVCenterLabelFromVM(ctx, m.client, webConsoleRequest,
		func(o metav1.Object) string {
			return o.(*vmopv1.VirtualMachineWebConsoleRequest).Spec.Name
		})
}
