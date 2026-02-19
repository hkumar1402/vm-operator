// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcscoped

import (
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/moid"
)

// VCZone wraps topologyv1.Zone with vCenter-scoped MoID access.
// All MoID accessor methods automatically filter and parse MoIDs to only include those
// belonging to the specified vCenter instance.
type VCZone struct {
	topologyv1.Zone
	vcenterUUID string
}

// NewVCZone creates a new vCenter-scoped Zone wrapper.
// The vcenterUUID must be non-empty (per-vCenter container context).
func NewVCZone(zone topologyv1.Zone, vcenterUUID string) VCZone {
	if vcenterUUID == "" {
		panic("vcenterUUID must be non-empty for VCZone - topology access should only occur in per-vCenter containers")
	}
	return VCZone{
		Zone:        zone,
		vcenterUUID: vcenterUUID,
	}
}

// GetManagedVMsFolder returns the parsed folder MoID for managed VMs.
// Returns empty string if the folder doesn't belong to this vCenter.
func (z VCZone) GetManagedVMsFolder() string {
	if z.Spec.ManagedVMs.FolderMoID == "" {
		return ""
	}
	if !moid.BelongsToVCenter(z.Spec.ManagedVMs.FolderMoID, z.vcenterUUID) {
		return ""
	}
	return moid.Parse(z.Spec.ManagedVMs.FolderMoID).MoID
}

// GetManagedVMsPools returns the filtered and parsed ResourcePool MoIDs for managed VMs.
// Returns only MoIDs belonging to this vCenter.
func (z VCZone) GetManagedVMsPools() []string {
	filtered := moid.FilterByVCenter(z.Spec.ManagedVMs.PoolMoIDs, z.vcenterUUID)
	result := make([]string, len(filtered))
	for i, parsed := range filtered {
		result[i] = parsed.MoID
	}
	return result
}

// GetNamespaceFolder returns the parsed folder MoID for the namespace.
// Returns empty string if the folder doesn't belong to this vCenter.
func (z VCZone) GetNamespaceFolder() string {
	if z.Spec.Namespace.FolderMoID == "" {
		return ""
	}
	if !moid.BelongsToVCenter(z.Spec.Namespace.FolderMoID, z.vcenterUUID) {
		return ""
	}
	return moid.Parse(z.Spec.Namespace.FolderMoID).MoID
}

// GetNamespacePools returns the filtered and parsed ResourcePool MoIDs for the namespace.
// Returns only MoIDs belonging to this vCenter.
func (z VCZone) GetNamespacePools() []string {
	filtered := moid.FilterByVCenter(z.Spec.Namespace.PoolMoIDs, z.vcenterUUID)
	result := make([]string, len(filtered))
	for i, parsed := range filtered {
		result[i] = parsed.MoID
	}
	return result
}

// GetVSpherePodsFolder returns the parsed folder MoID for vSphere pods.
// Returns empty string if the folder doesn't belong to this vCenter.
func (z VCZone) GetVSpherePodsFolder() string {
	if z.Spec.VSpherePods.FolderMoID == "" {
		return ""
	}
	if !moid.BelongsToVCenter(z.Spec.VSpherePods.FolderMoID, z.vcenterUUID) {
		return ""
	}
	return moid.Parse(z.Spec.VSpherePods.FolderMoID).MoID
}

// GetVSpherePodsPools returns the filtered and parsed ResourcePool MoIDs for vSphere pods.
// Returns only MoIDs belonging to this vCenter.
func (z VCZone) GetVSpherePodsPools() []string {
	filtered := moid.FilterByVCenter(z.Spec.VSpherePods.PoolMoIDs, z.vcenterUUID)
	result := make([]string, len(filtered))
	for i, parsed := range filtered {
		result[i] = parsed.MoID
	}
	return result
}

// BelongsToVCenter returns true if this Zone has any resources belonging to the vCenter.
func (z VCZone) BelongsToVCenter() bool {
	return z.GetManagedVMsFolder() != "" ||
		len(z.GetManagedVMsPools()) > 0 ||
		z.GetNamespaceFolder() != "" ||
		len(z.GetNamespacePools()) > 0 ||
		z.GetVSpherePodsFolder() != "" ||
		len(z.GetVSpherePodsPools()) > 0
}
