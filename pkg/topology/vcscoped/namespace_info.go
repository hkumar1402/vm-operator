// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcscoped

import (
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/moid"
)

// VCNamespaceInfo wraps topologyv1.NamespaceInfo with vCenter-scoped MoID access.
type VCNamespaceInfo struct {
	topologyv1.NamespaceInfo
	vcenterUUID string
}

// NewVCNamespaceInfo creates a new vCenter-scoped NamespaceInfo wrapper.
func NewVCNamespaceInfo(nsInfo topologyv1.NamespaceInfo, vcenterUUID string) VCNamespaceInfo {
	if vcenterUUID == "" {
		panic("vcenterUUID must be non-empty for VCNamespaceInfo - topology access should only occur in per-vCenter containers")
	}
	return VCNamespaceInfo{
		NamespaceInfo: nsInfo,
		vcenterUUID:   vcenterUUID,
	}
}

// GetFolderMoID returns the parsed folder MoID.
// Returns empty string if the folder doesn't belong to this vCenter.
func (n VCNamespaceInfo) GetFolderMoID() string {
	if n.FolderMoId == "" {
		return ""
	}
	if !moid.BelongsToVCenter(n.FolderMoId, n.vcenterUUID) {
		return ""
	}
	return moid.Parse(n.FolderMoId).MoID
}

// GetPoolMoIDs returns the filtered and parsed ResourcePool MoIDs.
// Returns only MoIDs belonging to this vCenter.
// Handles both deprecated PoolMoId field and new PoolMoIDs array.
func (n VCNamespaceInfo) GetPoolMoIDs() []string {
	var moIDs []string

	// Collect from both old and new fields
	if n.PoolMoId != "" {
		moIDs = append(moIDs, n.PoolMoId)
	}
	moIDs = append(moIDs, n.PoolMoIDs...)

	// Filter by vCenter and extract parsed MoIDs
	filtered := moid.FilterByVCenter(moIDs, n.vcenterUUID)
	result := make([]string, len(filtered))
	for i, parsed := range filtered {
		result[i] = parsed.MoID
	}
	return result
}

// GetFirstPoolMoID returns the first filtered and parsed ResourcePool MoID.
// Returns empty string if no pools belong to this vCenter.
func (n VCNamespaceInfo) GetFirstPoolMoID() string {
	pools := n.GetPoolMoIDs()
	if len(pools) == 0 {
		return ""
	}
	return pools[0]
}

// HasResources returns true if this namespace has any resources (folder or pools) for this vCenter.
func (n VCNamespaceInfo) HasResources() bool {
	return n.GetFolderMoID() != "" || len(n.GetPoolMoIDs()) > 0
}
