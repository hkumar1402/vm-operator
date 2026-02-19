// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package moid

import "strings"

// ParsedMoID represents a MoID with its associated vCenter UUID.
type ParsedMoID struct {
	// MoID is the vSphere Managed Object ID (e.g., "resgroup-123", "domain-c100").
	MoID string
	
	// VCenterUUID is the vCenter instance UUID extracted from the MoID format.
	// Empty string indicates legacy single-vCenter format.
	VCenterUUID string
}

// Parse extracts the MoID and vCenter UUID from the format "moid:vcenter-uuid".
// For backward compatibility, if no colon is present, the entire string is treated
// as the MoID with an empty VCenterUUID (legacy single-vCenter format).
// Only splits on the first colon to handle UUIDs that may contain colons.
//
// Examples:
//   - "resgroup-123:52f9b3e1-8d4a-4c3b-9a1e-2f7d8c5b4a3e" -> MoID: "resgroup-123", VCenterUUID: "52f9b3e1-8d4a-4c3b-9a1e-2f7d8c5b4a3e"
//   - "resgroup-123" -> MoID: "resgroup-123", VCenterUUID: ""
func Parse(moIDWithVC string) ParsedMoID {
	idx := strings.Index(moIDWithVC, ":")
	if idx > 0 {
		return ParsedMoID{
			MoID:        moIDWithVC[:idx],
			VCenterUUID: moIDWithVC[idx+1:],
		}
	}
	
	// Backward compatibility: no suffix means legacy single-vCenter format
	return ParsedMoID{
		MoID:        moIDWithVC,
		VCenterUUID: "",
	}
}

// FilterByVCenter returns only MoIDs belonging to the specified vCenter UUID.
// MoIDs with empty VCenterUUID (legacy format) are included for backward compatibility.
//
// Example:
//   moIDs := []string{
//       "resgroup-10:vc1-uuid",
//       "resgroup-20:vc2-uuid",
//       "resgroup-30", // legacy format
//   }
//   filtered := FilterByVCenter(moIDs, "vc1-uuid")
//   // Returns: [ParsedMoID{MoID: "resgroup-10", VCenterUUID: "vc1-uuid"}, ParsedMoID{MoID: "resgroup-30", VCenterUUID: ""}]
func FilterByVCenter(moIDs []string, vcenterUUID string) []ParsedMoID {
	var filtered []ParsedMoID
	for _, id := range moIDs {
		parsed := Parse(id)
		// Include if it matches this vCenter, or if it's legacy format (empty UUID)
		if parsed.VCenterUUID == "" || parsed.VCenterUUID == vcenterUUID {
			filtered = append(filtered, parsed)
		}
	}
	return filtered
}

// BelongsToVCenter checks if a single MoID belongs to the specified vCenter.
// Returns true if the MoID matches the vCenter UUID or is in legacy format (empty UUID).
// Returns false if the MoID explicitly belongs to a different vCenter.
//
// Example:
//   BelongsToVCenter("resgroup-10:vc1-uuid", "vc1-uuid")  // true
//   BelongsToVCenter("resgroup-10:vc2-uuid", "vc1-uuid")  // false
//   BelongsToVCenter("resgroup-10", "vc1-uuid")           // true (legacy)
func BelongsToVCenter(moID, vcenterUUID string) bool {
	parsed := Parse(moID)
	return parsed.VCenterUUID == "" || parsed.VCenterUUID == vcenterUUID
}
