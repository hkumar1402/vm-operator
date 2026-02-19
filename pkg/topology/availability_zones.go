// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package topology

import (
	"context"
	"errors"
	"fmt"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/topology/vcscoped"
)

var (
	// ErrNoAvailabilityZones occurs when no availability zones are detected.
	ErrNoAvailabilityZones = errors.New("no availability zones")

	ErrNoZones = errors.New("no zones in specified namespace")
)

// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=availabilityzones,verbs=get;list;watch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=availabilityzones/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=zones,verbs=get;list;watch
// +kubebuilder:rbac:groups=topology.tanzu.vmware.com,resources=zones/status,verbs=get;list;watch

// LookupZoneForClusterMoID returns the zone for the given Cluster MoID.
// The clusterMoID parameter is a plain MoID (e.g., "domain-c100") without vCenter UUID suffix.
// This function runs in per-vCenter container context and only considers clusters belonging
// to the current vCenter.
func LookupZoneForClusterMoID(
	ctx context.Context,
	client ctrlclient.Client,
	clusterMoID string) (string, error) {

	availabilityZones, err := GetAvailabilityZones(ctx, client)
	if err != nil {
		return "", err
	}

	vcenterUUID := pkgcfg.FromContext(ctx).VCenterInstanceUUID

	for _, az := range availabilityZones {
		// Use wrapper method to get filtered cluster MoIDs
		for _, azClusterMoID := range az.GetClusterMoIDs() {
			if azClusterMoID == clusterMoID {
				return az.Name, nil
			}
		}
	}

	return "", fmt.Errorf("failed to find availability zone for cluster MoID %s in vCenter %s", clusterMoID, vcenterUUID)
}

// GetNamespaceFolderAndRPMoID returns the Folder and ResourcePool MoID for the zone and namespace.
// The returned MoIDs are automatically filtered and parsed by the wrapper types.
// Returns error if no matching MoIDs are found for this vCenter.
func GetNamespaceFolderAndRPMoID(
	ctx context.Context,
	client ctrlclient.Client,
	availabilityZoneName, namespace string) (string, string, error) {

	vcenterUUID := pkgcfg.FromContext(ctx).VCenterInstanceUUID

	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		zone, err := GetZone(ctx, client, availabilityZoneName, namespace)
		if err != nil {
			return "", "", err
		}

		folderMoID := zone.GetManagedVMsFolder()
		if folderMoID == "" {
			return "", "", fmt.Errorf("folder MoID does not belong to vCenter %s", vcenterUUID)
		}

		pools := zone.GetManagedVMsPools()
		if len(pools) == 0 {
			return folderMoID, "", nil
		}

		return folderMoID, pools[0], nil
	}

	availabilityZone, err := GetAvailabilityZone(ctx, client, availabilityZoneName)
	if err != nil {
		return "", "", err
	}

	nsInfo, ok := availabilityZone.GetNamespaceInfo(namespace)
	if !ok {
		return "", "", fmt.Errorf("availability zone %q missing info for namespace %s",
			availabilityZoneName, namespace)
	}

	folderMoID := nsInfo.GetFolderMoID()
	if folderMoID == "" {
		return "", "", fmt.Errorf("folder MoID does not belong to vCenter %s", vcenterUUID)
	}

	poolMoID := nsInfo.GetFirstPoolMoID()
	if poolMoID == "" {
		return "", "", fmt.Errorf("no resource pool MoIDs belong to vCenter %s", vcenterUUID)
	}

	return folderMoID, poolMoID, nil
}

// GetNamespaceFolderAndRPMoIDs returns the Folder and ResourcePool MoIDs for the namespace, across all zones.
// The returned MoIDs are automatically filtered and parsed by the wrapper types.
func GetNamespaceFolderAndRPMoIDs(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string) (string, []string, error) {

	var folderMoID string
	var rpMoIDs []string

	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		zones, err := GetZones(ctx, client, namespace)
		// If no Zones found in namespace, do not return err.
		if err != nil && !errors.Is(err, ErrNoZones) {
			return "", nil, err
		}

		for _, zone := range zones {
			// Use wrapper methods to get filtered MoIDs
			if folderMoID == "" {
				folderMoID = zone.GetManagedVMsFolder()
			}
			rpMoIDs = append(rpMoIDs, zone.GetManagedVMsPools()...)
		}

		return folderMoID, rpMoIDs, nil
	}

	availabilityZones, err := GetAvailabilityZones(ctx, client)
	if err != nil {
		return "", nil, err
	}

	for _, az := range availabilityZones {
		nsInfo, ok := az.GetNamespaceInfo(namespace)
		if !ok {
			continue
		}

		// Use wrapper methods to get filtered MoIDs
		if folderMoID == "" {
			folderMoID = nsInfo.GetFolderMoID()
		}
		rpMoIDs = append(rpMoIDs, nsInfo.GetPoolMoIDs()...)
	}

	return folderMoID, rpMoIDs, nil
}

// GetNamespaceFolderMoID returns the FolderMoID for the namespace.
// The returned MoID is automatically filtered and parsed by the wrapper types.
func GetNamespaceFolderMoID(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string) (string, error) {

	vcenterUUID := pkgcfg.FromContext(ctx).VCenterInstanceUUID

	if pkgcfg.FromContext(ctx).Features.WorkloadDomainIsolation {
		zones, err := GetZones(ctx, client, namespace)
		// If no Zones found in namespace, do not return err here.
		if err != nil && !errors.Is(err, ErrNoZones) {
			return "", err
		}
		// Note that the Folder is VC-scoped, but we store the Folder MoID in each Zone CR
		// so we can return the first match that belongs to this vCenter.
		for _, zone := range zones {
			if folderMoID := zone.GetManagedVMsFolder(); folderMoID != "" {
				return folderMoID, nil
			}
		}
		return "", fmt.Errorf("unable to get FolderMoID for namespace %s and vCenter %s", namespace, vcenterUUID)
	}

	availabilityZones, err := GetAvailabilityZones(ctx, client)
	if err != nil {
		return "", err
	}

	// Note that the Folder is VC-scoped, but we store the Folder MoID in each Zone CR
	// so we can return the first match that belongs to this vCenter.
	for _, az := range availabilityZones {
		nsInfo, ok := az.GetNamespaceInfo(namespace)
		if ok {
			if folderMoID := nsInfo.GetFolderMoID(); folderMoID != "" {
				return folderMoID, nil
			}
		}
	}

	return "", fmt.Errorf("unable to get FolderMoID for namespace %s and vCenter %s", namespace, vcenterUUID)
}

// GetAvailabilityZones returns a list of vCenter-scoped AvailabilityZone resources.
func GetAvailabilityZones(
	ctx context.Context,
	client ctrlclient.Client) ([]vcscoped.VCAvailabilityZone, error) {

	availabilityZoneList := &topologyv1.AvailabilityZoneList{}
	if err := client.List(ctx, availabilityZoneList); err != nil {
		return nil, err
	}

	if len(availabilityZoneList.Items) == 0 {
		return nil, ErrNoAvailabilityZones
	}

	vcenterUUID := pkgcfg.FromContext(ctx).VCenterInstanceUUID
	result := make([]vcscoped.VCAvailabilityZone, len(availabilityZoneList.Items))
	for i, az := range availabilityZoneList.Items {
		result[i] = vcscoped.NewVCAvailabilityZone(az, vcenterUUID)
	}

	return result, nil
}

// GetAvailabilityZone returns a vCenter-scoped named AvailabilityZone resource.
func GetAvailabilityZone(
	ctx context.Context,
	client ctrlclient.Client,
	availabilityZoneName string) (vcscoped.VCAvailabilityZone, error) {

	var availabilityZone topologyv1.AvailabilityZone
	err := client.Get(ctx, ctrlclient.ObjectKey{Name: availabilityZoneName}, &availabilityZone)
	if err != nil {
		return vcscoped.VCAvailabilityZone{}, err
	}

	vcenterUUID := pkgcfg.FromContext(ctx).VCenterInstanceUUID
	return vcscoped.NewVCAvailabilityZone(availabilityZone, vcenterUUID), nil
}

// GetZones returns a list of vCenter-scoped Zone resources in a namespace.
func GetZones(
	ctx context.Context,
	client ctrlclient.Client,
	namespace string) ([]vcscoped.VCZone, error) {

	zoneList := &topologyv1.ZoneList{}
	if err := client.List(ctx, zoneList, ctrlclient.InNamespace(namespace)); err != nil {
		return nil, err
	}

	if len(zoneList.Items) == 0 {
		return nil, ErrNoZones
	}

	vcenterUUID := pkgcfg.FromContext(ctx).VCenterInstanceUUID
	result := make([]vcscoped.VCZone, len(zoneList.Items))
	for i, zone := range zoneList.Items {
		result[i] = vcscoped.NewVCZone(zone, vcenterUUID)
	}

	return result, nil
}

// GetZone returns a vCenter-scoped namespaced Zone resource.
func GetZone(
	ctx context.Context,
	client ctrlclient.Client,
	zoneName string,
	namespace string) (vcscoped.VCZone, error) {

	var zone topologyv1.Zone
	err := client.Get(ctx, ctrlclient.ObjectKey{Name: zoneName, Namespace: namespace}, &zone)
	if err != nil {
		return vcscoped.VCZone{}, err
	}

	vcenterUUID := pkgcfg.FromContext(ctx).VCenterInstanceUUID
	return vcscoped.NewVCZone(zone, vcenterUUID), nil
}
