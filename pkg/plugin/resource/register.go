// Package resource registers the Proxmox resource capability.
package resource

import (
	"context"
	"fmt"

	"github.com/omniviewdev/plugin-sdk/pkg/sdk"
	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"
	v1plugin "github.com/omniviewdev/plugin-sdk/pkg/v1/resource/plugin"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

const (
	group   = "proxmox"
	version = "v1"

	categoryCluster = "Cluster"
	categoryCompute = "Compute"
	categoryCeph    = "Ceph"
)

var (
	metaNode = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "Node", Label: "Nodes", Category: categoryCluster,
		Description: "Proxmox VE cluster members.",
	}
	metaStorage = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "Storage", Label: "Storage", Category: categoryCluster,
		Description: "Storage as seen by each node.",
	}
	metaVM = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "VirtualMachine", Label: "Virtual Machines", Category: categoryCompute,
		Description: "QEMU/KVM guests.",
	}
	metaContainer = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "Container", Label: "Containers", Category: categoryCompute,
		Description: "LXC guests.",
	}
	metaCephCluster = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "CephCluster", Label: "Ceph Status", Category: categoryCeph,
		Description: "Health and capacity of the Ceph cluster (ceph status).",
	}
	metaCephPool = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "CephPool", Label: "Pools", Category: categoryCeph,
		Description: "Ceph pools.",
	}
	metaCephOSD = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "CephOSD", Label: "OSDs", Category: categoryCeph,
		Description: "Ceph object storage daemons.",
	}
	metaCephMonitor = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "CephMonitor", Label: "Monitors", Category: categoryCeph,
		Description: "Ceph monitor daemons.",
	}
	metaCephManager = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "CephManager", Label: "Managers", Category: categoryCeph,
		Description: "Ceph manager daemons.",
	}
	metaCephMDS = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "CephMetadataServer", Label: "Metadata Servers", Category: categoryCeph,
		Description: "CephFS metadata server daemons.",
	}
	metaCephFS = sdkresource.ResourceMeta{
		Group: group, Version: version, Kind: "CephFilesystem", Label: "CephFS", Category: categoryCeph,
		Description: "CephFS file systems.",
	}
)

// Register adds the resource capability to plugin. Registrations carry no
// ResourceDefinition: no RPC delivers definitions to the UI, which owns the
// table columns (ui/src/utils/columns.ts). Ceph kinds are registered for
// every cluster; one without Ceph answers them with CEPH_UNAVAILABLE.
func Register(plugin *sdk.Plugin) {
	cfg := sdkresource.ResourcePluginConfig[proxmox.Client]{
		Connections: &connectionProvider{},
		Resources: []sdkresource.ResourceRegistration[proxmox.Client]{
			{Meta: metaNode, Resourcer: clusterKind("node", proxmox.ResourceTypeNode, "node")},
			{Meta: metaStorage, Resourcer: clusterKind("storage", proxmox.ResourceTypeStorage, "storage")},
			{Meta: metaVM, Resourcer: clusterKind("virtual machine", proxmox.ResourceTypeVM, "qemu")},
			{Meta: metaContainer, Resourcer: clusterKind("container", proxmox.ResourceTypeVM, "lxc")},
			{Meta: metaCephCluster, Resourcer: cephKind("Ceph cluster", (*proxmox.Client).CephStatus)},
			{Meta: metaCephPool, Resourcer: cephKind("Ceph pool", (*proxmox.Client).CephPools)},
			{Meta: metaCephOSD, Resourcer: cephKind("Ceph OSD", (*proxmox.Client).CephOSDs)},
			{Meta: metaCephMonitor, Resourcer: cephKind("Ceph monitor", (*proxmox.Client).CephMonitors)},
			{Meta: metaCephManager, Resourcer: cephKind("Ceph manager", (*proxmox.Client).CephManagers)},
			{Meta: metaCephMDS, Resourcer: cephKind("Ceph metadata server", (*proxmox.Client).CephMetadataServers)},
			{Meta: metaCephFS, Resourcer: cephKind("CephFS", (*proxmox.Client).CephFilesystems)},
		},
		Groups: []sdkresource.ResourceGroup{{
			ID:          group,
			Name:        "Proxmox VE",
			Description: "Proxmox VE cluster resources",
			Resources: map[string][]sdkresource.ResourceMeta{
				version: {
					metaNode, metaStorage, metaVM, metaContainer,
					metaCephCluster, metaCephPool, metaCephOSD, metaCephMonitor, metaCephManager, metaCephMDS, metaCephFS,
				},
			},
		}},
	}

	controller, err := sdkresource.BuildResourceController(context.Background(), cfg)
	if err != nil {
		panic(fmt.Sprintf("build proxmox resource controller: %v", err))
	}
	plugin.RegisterCapability("resource", &v1plugin.GRPCPlugin{
		Impl:             controller,
		SettingsProvider: plugin.SettingsProvider,
	})
}
