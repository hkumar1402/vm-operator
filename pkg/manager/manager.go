// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/config"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	// Load the GCP authentication plug-in.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	imgregv1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha2"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	appv1a1 "github.com/vmware-tanzu/vm-operator/external/appplatform/api/v1alpha1"
	byokv1 "github.com/vmware-tanzu/vm-operator/external/byok/api/v1alpha1"
	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	infrav1 "github.com/vmware-tanzu/vm-operator/external/infra/api/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
	topologyv1 "github.com/vmware-tanzu/vm-operator/external/tanzu-topology/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	vspherepolv1 "github.com/vmware-tanzu/vm-operator/external/vsphere-policy/api/v1alpha1"

	vmopapi "github.com/vmware-tanzu/vm-operator/api"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// Manager is a VM Operator controller manager.
type Manager interface {
	ctrlmgr.Manager

	// GetContext returns the controller manager's pkgctx.
	GetContext() *pkgctx.ControllerManagerContext
}

// New returns a new VM Operator controller manager.
func New(ctx context.Context, opts Options) (Manager, error) {
	// Ensure the default options are set.
	opts.defaults()

	//
	// External -- Kubernetes
	//
	_ = appsv1.AddToScheme(opts.Scheme)
	_ = clientgoscheme.AddToScheme(opts.Scheme)
	_ = capv1.AddToScheme(opts.Scheme)

	//
	// External -- VMware
	//
	_ = ncpv1alpha1.AddToScheme(opts.Scheme)
	_ = cnsv1alpha1.AddToScheme(opts.Scheme)
	_ = netopv1alpha1.AddToScheme(opts.Scheme)
	_ = topologyv1.AddToScheme(opts.Scheme)
	_ = imgregv1a1.AddToScheme(opts.Scheme)
	_ = spqv1.AddToScheme(opts.Scheme)
	_ = byokv1.AddToScheme(opts.Scheme)
	_ = vspherepolv1.AddToScheme(opts.Scheme)
	_ = appv1a1.AddToScheme(opts.Scheme)
	_ = infrav1.AddToScheme(opts.Scheme)

	//
	// VM Op
	//
	_ = vmopapi.AddToScheme(opts.Scheme)

	if pkgcfg.FromContext(ctx).Features.InventoryContentLibrary {
		_ = imgregv1.AddToScheme(opts.Scheme)
	}

	if pkgcfg.FromContext(ctx).NetworkProviderType == pkgcfg.NetworkProviderTypeVPC {
		_ = vpcv1alpha1.AddToScheme(opts.Scheme)
	}

	// Build cache options with label-based filtering for per-vCenter containers
	cacheOpts := cache.Options{
		DefaultNamespaces: GetNamespaceCacheConfigs(opts.WatchNamespace),
		DefaultTransform:  cache.TransformStripManagedFields(),
		SyncPeriod:        &opts.SyncPeriod,
	}

	// Per-vCenter containers: Filter cached resources by vCenter label
	// This reduces memory usage by only caching resources this container will reconcile.
	// Assumes external migration workflow has labeled all existing resources.
	// Global resources (Zone, VirtualMachineClass, etc.) are not filtered.
	if config := pkgcfg.FromContext(ctx); config.IsPerVCenterMode() {
		cacheOpts.ByObject = getPerVCenterCacheConfig(config.VCenterInstanceUUID)
	}

	// Build the controller manager.
	mgr, err := ctrlmgr.New(opts.KubeConfig, ctrlmgr.Options{
		Scheme: opts.Scheme,
		Cache:  cacheOpts,
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					// An informer is created for each watched resource. Due to the
					// number of ConfigMap and Secret resources that may exist,
					// watching each one can result in VM Operator being terminated
					// due to an out-of-memory error, i.e. OOMKill. To avoid this
					// outcome, ConfigMap and Secret resources are not cached.
					&corev1.ConfigMap{},
					&corev1.Secret{},

					// The pkg/exit.Restart function gets a Deployment resource
					// in order to patch it to restart the pods in the
					// deployment when capabilities have changed.
					// Capabilities do not change often enough to warrant
					// caching the Deployment resource, and thus there is no
					// reason to cache Deployment resources as nothing else in
					// VM Operator gets them.
					&appsv1.Deployment{},
				},
			},
		},
		Controller: ctrlcfg.Controller{
			UsePriorityQueue: &opts.UsePriorityQueue,
		},
		Metrics: metricsserver.Options{
			BindAddress: opts.MetricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			CertDir: opts.WebhookSecretVolumeMountPath,
			Port:    opts.WebhookServiceContainerPort,
		}),
		HealthProbeBindAddress:  opts.HealthProbeBindAddress,
		PprofBindAddress:        opts.PprofBindAddress,
		LeaderElection:          opts.LeaderElectionEnabled,
		LeaderElectionID:        opts.LeaderElectionID,
		LeaderElectionNamespace: opts.PodNamespace,
		NewCache:                opts.NewCache,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create manager: %w", err)
	}

	// Prefix the logger with the pod name.
	logger := opts.Logger.WithName(opts.PodName)

	// Build the controller manager pkgctx.
	controllerManagerContext := &pkgctx.ControllerManagerContext{
		Context:                         logr.NewContext(ctx, logger),
		Namespace:                       opts.PodNamespace,
		Name:                            opts.PodName,
		ServiceAccountName:              opts.PodServiceAccountName,
		LeaderElectionID:                opts.LeaderElectionID,
		LeaderElectionNamespace:         opts.PodNamespace,
		MaxConcurrentReconciles:         opts.MaxConcurrentReconciles,
		Logger:                          logger,
		Recorder:                        record.New(mgr.GetEventRecorderFor(fmt.Sprintf("%s/%s", opts.PodNamespace, opts.PodName))),
		ContainerNode:                   opts.ContainerNode,
		SyncPeriod:                      opts.SyncPeriod,
		EnableWebhookClientVerification: opts.EnableWebhookClientVerification,
	}

	if err := opts.InitializeProviders(controllerManagerContext, mgr); err != nil {
		return nil, err
	}

	// Add the requested items to the manager.
	if err := opts.AddToManager(controllerManagerContext, mgr); err != nil {
		return nil, fmt.Errorf("failed to add resources to the manager: %w", err)
	}

	return &manager{
		Manager: mgr,
		ctx:     controllerManagerContext,
	}, nil
}

type manager struct {
	ctrlmgr.Manager
	ctx *pkgctx.ControllerManagerContext
}

func (m *manager) GetContext() *pkgctx.ControllerManagerContext {
	return m.ctx
}

// getPerVCenterCacheConfig returns cache configuration for per-vCenter containers.
// It filters per-vCenter resources by label to reduce memory usage.
// Global resources (Zone, VirtualMachineClass, etc.) are NOT filtered - they must
// be cached by all containers.
func getPerVCenterCacheConfig(vcenterUUID string) map[client.Object]cache.ByObject {
	// Create label selector for this vCenter's resources
	labelSelector := labels.SelectorFromSet(labels.Set{
		constants.VCenterIDLabel: vcenterUUID,
	})

	return map[client.Object]cache.ByObject{
		// Per-vCenter resources: Only cache resources with matching vCenter label
		// These resources are labeled by mutation webhooks during creation

		// VirtualMachine: Labeled by mutation webhook (random assignment)
		&vmopv1.VirtualMachine{}: {
			Label: labelSelector,
		},
		// VirtualMachineSnapshot: Labeled by mutation webhook (copied from parent VM)
		&vmopv1.VirtualMachineSnapshot{}: {
			Label: labelSelector,
		},
		// VirtualMachineWebConsoleRequest: Labeled by mutation webhook (copied from referenced VM)
		&vmopv1.VirtualMachineWebConsoleRequest{}: {
			Label: labelSelector,
		},
		// VirtualMachinePublishRequest: Labeled by mutation webhook (copied from source VM)
		&vmopv1.VirtualMachinePublishRequest{}: {
			Label: labelSelector,
		},
		// ContentLibraryItem: Labeled by image-registry-operator with vCenter UUID
		&imgregv1a1.ContentLibraryItem{}: {
			Label: labelSelector,
		},
		&imgregv1.ContentLibraryItem{}: {
			Label: labelSelector,
		},

		// Note: The following resources are NOT filtered and will be cached by all containers:
		// - Zone: Global resource (no label, uses MoID filtering in controller)
		// - AvailabilityZone: Global resource (no label, uses MoID filtering in controller)
		// - VirtualMachineClass: Shared resource (no label, used across all vCenters)
		// - VirtualMachineImage: Shared resource (no label when from shared content library)
		// - ClusterVirtualMachineImage: Shared resource (no label when from shared content library)
		// - EncryptionClass: Global resource (no label)
		// - StorageClass: Per-vCenter but managed externally (labeled by CSI driver)
	}
}
