// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcscoped

import (
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/moid"
)

// VCAvailabilityZone wraps topologyv1.AvailabilityZone with vCenter-scoped MoID access.
// All MoID accessor methods automatically filter and parse MoIDs to only include those
// belonging to the specified vCenter instance.
type VCAvailabilityZone struct {
	topologyv1.AvailabilityZone
	vcenterUUID string
}

// NewVCAvailabilityZone creates a new vCenter-scoped AvailabilityZone wrapper.
// The vcenterUUID must be non-empty (per-vCenter container context).
func NewVCAvailabilityZone(az topologyv1.AvailabilityZone, vcenterUUID string) VCAvailabilityZone {
	if vcenterUUID == "" {
		panic("vcenterUUID must be non-empty for VCAvailabilityZone - topology access should only occur in per-vCenter containers")
	}
	return VCAvailabilityZone{
		AvailabilityZone: az,
		vcenterUUID:      vcenterUUID,
	}
}

// GetClusterMoIDs returns the filtered and parsed cluster MoIDs that belong to this vCenter.
// Returns only the MoID portion (e.g., "domain-c100") without vCenter UUID suffix.
func (az VCAvailabilityZone) GetClusterMoIDs() []string {
	var moIDs []string

	// Collect all cluster MoIDs (both deprecated single field and new array)
	if az.Spec.ClusterComputeResourceMoId != "" {
		moIDs = append(moIDs, az.Spec.ClusterComputeResourceMoId)
	}
	moIDs = append(moIDs, az.Spec.ClusterComputeResourceMoIDs...)

	// Filter by vCenter and extract parsed MoIDs
	filtered := moid.FilterByVCenter(moIDs, az.vcenterUUID)
	result := make([]string, len(filtered))
	for i, parsed := range filtered {
		result[i] = parsed.MoID
	}
	return result
}

// GetNamespaceInfo returns the NamespaceInfo for the given namespace with filtered MoIDs.
// Returns false if the namespace is not found or has no resources for this vCenter.
func (az VCAvailabilityZone) GetNamespaceInfo(namespace string) (VCNamespaceInfo, bool) {
	nsInfo, ok := az.Spec.Namespaces[namespace]
	if !ok {
		return VCNamespaceInfo{}, false
	}

	return NewVCNamespaceInfo(nsInfo, az.vcenterUUID), true
}

// BelongsToVCenter returns true if this AvailabilityZone has any resources belonging to the vCenter.
func (az VCAvailabilityZone) BelongsToVCenter() bool {
	// Check if any cluster MoIDs belong to this vCenter
	if len(az.GetClusterMoIDs()) > 0 {
		return true
	}

	// Check if any namespace has resources for this vCenter
	for _, nsInfo := range az.Spec.Namespaces {
		vcNsInfo := NewVCNamespaceInfo(nsInfo, az.vcenterUUID)
		if vcNsInfo.HasResources() {
			return true
		}
	}

	return false
}
