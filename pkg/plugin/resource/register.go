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
// table columns (ui/src/utils/columns.ts).
//
// The Ceph kinds are not static registrations: cephDiscovery adds their types
// to a connection only when its cluster answers `ceph status`, so a cluster
// without Ceph has no Ceph section. Exact-key Patterns route them to their
// resourcers. Discovery lists the static kinds as well, or the SDK would not
// watch them.
func Register(plugin *sdk.Plugin) {
	ceph := []sdkresource.ResourceRegistration[proxmox.Client]{
		{Meta: metaCephCluster, Resourcer: cephKind(metaCephCluster, "Ceph cluster", (*proxmox.Client).CephStatus)},
		{Meta: metaCephPool, Resourcer: cephKind(metaCephPool, "Ceph pool", (*proxmox.Client).CephPools)},
		{Meta: metaCephOSD, Resourcer: cephKind(metaCephOSD, "Ceph OSD", (*proxmox.Client).CephOSDs)},
		{Meta: metaCephMonitor, Resourcer: cephKind(metaCephMonitor, "Ceph monitor", (*proxmox.Client).CephMonitors)},
		{Meta: metaCephManager, Resourcer: cephKind(metaCephManager, "Ceph manager", (*proxmox.Client).CephManagers)},
		{Meta: metaCephMDS, Resourcer: cephKind(metaCephMDS, "Ceph metadata server", (*proxmox.Client).CephMetadataServers)},
		{Meta: metaCephFS, Resourcer: cephKind(metaCephFS, "CephFS", (*proxmox.Client).CephFilesystems)},
	}
	cephPatterns := make(map[string]sdkresource.Resourcer[proxmox.Client], len(ceph))
	cephMetas := make([]sdkresource.ResourceMeta, len(ceph))
	for i, reg := range ceph {
		cephPatterns[reg.Meta.Key()] = reg.Resourcer
		cephMetas[i] = reg.Meta
	}

	static := []sdkresource.ResourceRegistration[proxmox.Client]{
		{Meta: metaNode, Resourcer: clusterKind(metaNode, "node", proxmox.ResourceTypeNode, "node")},
		{Meta: metaStorage, Resourcer: clusterKind(metaStorage, "storage", proxmox.ResourceTypeStorage, "storage")},
		{Meta: metaVM, Resourcer: guestKind(metaVM, "virtual machine", "qemu")},
		{Meta: metaContainer, Resourcer: guestKind(metaContainer, "container", "lxc")},
	}
	staticMetas := make([]sdkresource.ResourceMeta, len(static))
	for i, reg := range static {
		staticMetas[i] = reg.Meta
	}

	cfg := sdkresource.ResourcePluginConfig[proxmox.Client]{
		Connections: &connectionProvider{},
		Resources:   static,
		Patterns:    cephPatterns,
		Discovery:   &cephDiscovery{static: staticMetas, ceph: cephMetas},
		Groups: []sdkresource.ResourceGroup{{
			ID:          group,
			Name:        "Proxmox VE",
			Description: "Proxmox VE cluster resources",
			Resources: map[string][]sdkresource.ResourceMeta{
				version: staticMetas,
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
