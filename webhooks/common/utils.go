// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
	"fmt"
	"sort"
	"strings"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
)

const (
	DefaultImagePublishContentLibraryLabelKey = "imageregistry.vmware.com/default"
)

// RetrieveDefaultImagePublishContentLibrary returns the default content library with the
// "imageregistry.vmware.com/default" label. A NotFound error is returned if no default content library
// was found. A BadRequest error is returned if more than one default content libraries were found.
func RetrieveDefaultImagePublishContentLibrary(ctx context.Context, c ctrlclient.Client, namespace string) (
	*imgregv1a1.ContentLibrary, error) {
	clList := &imgregv1a1.ContentLibraryList{}
	if err := c.List(ctx,
		clList,
		ctrlclient.InNamespace(namespace),
		ctrlclient.MatchingLabels{DefaultImagePublishContentLibraryLabelKey: ""},
	); err != nil {
		return nil, err
	}

	if len(clList.Items) == 0 {
		return nil, apierrors.NewNotFound(schema.GroupResource{
			Group:    imgregv1a1.GroupVersion.Group,
			Resource: "ContentLibrary",
		}, "")
	}

	if len(clList.Items) > 1 {
		clNames := make([]string, len(clList.Items))
		for i, cl := range clList.Items {
			clNames[i] = cl.Name
		}
		sort.Strings(clNames)
		return nil, apierrors.NewBadRequest(
			fmt.Sprintf("more than one default ContentLibrary found: %s", strings.Join(clNames, ", ")))
	}

	return &clList.Items[0], nil
}

// CopyVCenterLabelFromVM copies the vmoperator.vmware.com/vcenter-id label from the VM named
// by getVMName(obj) onto obj. Returns true if the label was copied. Returns false without error
// if obj already carries the label, if getVMName returns "", or if the VM is not found.
func CopyVCenterLabelFromVM(
	ctx context.Context,
	client ctrlclient.Client,
	obj metav1.Object,
	getVMName func(metav1.Object) string,
) (bool, error) {

	if obj.GetLabels()[constants.VCenterIDLabel] != "" {
		return false, nil
	}

	vmName := getVMName(obj)
	if vmName == "" {
		return false, nil
	}

	vm := &vmopv1.VirtualMachine{}
	if err := client.Get(ctx, ctrlclient.ObjectKey{Name: vmName, Namespace: obj.GetNamespace()}, vm); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get VM %s/%s: %w", obj.GetNamespace(), vmName, err)
	}

	vcenterID := vm.Labels[constants.VCenterIDLabel]
	if vcenterID == "" {
		return false, nil
	}

	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[constants.VCenterIDLabel] = vcenterID
	obj.SetLabels(labels)

	return true, nil
}

// ConvertFieldErrorsToStrings returns a list of error messages from the field.ErrorList's non nil errors.
func ConvertFieldErrorsToStrings(fieldErrs field.ErrorList) []string {
	var validationErrs []string
	for _, fieldErr := range fieldErrs {
		if fieldErr != nil {
			validationErrs = append(validationErrs, fieldErr.Error())
		}
	}
	return validationErrs
}
