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
)

// Register adds the resource capability to plugin. Registrations carry no
// ResourceDefinition: no RPC delivers definitions to the UI, which owns the
// table columns (ui/src/utils/columns.ts).
func Register(plugin *sdk.Plugin) {
	cfg := sdkresource.ResourcePluginConfig[proxmox.Client]{
		Connections: &connectionProvider{},
		Resources: []sdkresource.ResourceRegistration[proxmox.Client]{
			{Meta: metaNode, Resourcer: &clusterResourcer{apiType: proxmox.ResourceTypeNode, entryType: "node"}},
			{Meta: metaStorage, Resourcer: &clusterResourcer{apiType: proxmox.ResourceTypeStorage, entryType: "storage"}},
			{Meta: metaVM, Resourcer: &clusterResourcer{apiType: proxmox.ResourceTypeVM, entryType: "qemu"}},
			{Meta: metaContainer, Resourcer: &clusterResourcer{apiType: proxmox.ResourceTypeVM, entryType: "lxc"}},
		},
		Groups: []sdkresource.ResourceGroup{{
			ID:          group,
			Name:        "Proxmox VE",
			Description: "Proxmox VE cluster resources",
			Resources: map[string][]sdkresource.ResourceMeta{
				version: {metaNode, metaStorage, metaVM, metaContainer},
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
