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

var statusColors = map[string]string{
	"online": "success", "running": "success", "available": "success",
	"offline": "danger", "stopped": "neutral", "unknown": "warning",
	"paused": "warning", "*": "neutral",
}

func col(id, header, accessor string) sdkresource.ColumnDefinition {
	return sdkresource.ColumnDefinition{ID: id, Header: header, Accessors: accessor}
}

func bytesCol(id, header, accessor string) sdkresource.ColumnDefinition {
	c := col(id, header, accessor)
	c.Formatter = "BYTES"
	c.Alignment = "RIGHT"
	return c
}

func statusCol() sdkresource.ColumnDefinition {
	c := col("status", "Status", "status")
	c.ColorMap = statusColors
	return c
}

func guestDefinition() *sdkresource.ResourceDefinition {
	return &sdkresource.ResourceDefinition{
		IDAccessor: "id",
		ColumnDefs: []sdkresource.ColumnDefinition{
			col("vmid", "VMID", "vmid"),
			col("name", "Name", "name"),
			col("node", "Node", "node"),
			statusCol(),
			col("maxcpu", "vCPUs", "maxcpu"),
			bytesCol("mem", "Memory", "mem"),
			bytesCol("maxmem", "Max memory", "maxmem"),
			bytesCol("maxdisk", "Disk", "maxdisk"),
			col("tags", "Tags", "tags"),
		},
	}
}

func registrations() []sdkresource.ResourceRegistration[proxmox.Client] {
	return []sdkresource.ResourceRegistration[proxmox.Client]{
		{
			Meta:      metaNode,
			Resourcer: &clusterResourcer{apiType: proxmox.ResourceTypeNode, entryType: "node"},
			Definition: &sdkresource.ResourceDefinition{
				IDAccessor: "id",
				ColumnDefs: []sdkresource.ColumnDefinition{
					col("node", "Node", "node"),
					statusCol(),
					col("maxcpu", "CPUs", "maxcpu"),
					bytesCol("mem", "Memory", "mem"),
					bytesCol("maxmem", "Max memory", "maxmem"),
					bytesCol("maxdisk", "Root disk", "maxdisk"),
				},
			},
		},
		{
			Meta:      metaStorage,
			Resourcer: &clusterResourcer{apiType: proxmox.ResourceTypeStorage, entryType: "storage"},
			Definition: &sdkresource.ResourceDefinition{
				IDAccessor: "id",
				ColumnDefs: []sdkresource.ColumnDefinition{
					col("storage", "Storage", "storage"),
					col("node", "Node", "node"),
					statusCol(),
					col("plugintype", "Type", "plugintype"),
					col("content", "Content", "content"),
					bytesCol("disk", "Used", "disk"),
					bytesCol("maxdisk", "Size", "maxdisk"),
				},
			},
		},
		{
			Meta:       metaVM,
			Resourcer:  &clusterResourcer{apiType: proxmox.ResourceTypeVM, entryType: "qemu"},
			Definition: guestDefinition(),
		},
		{
			Meta:       metaContainer,
			Resourcer:  &clusterResourcer{apiType: proxmox.ResourceTypeVM, entryType: "lxc"},
			Definition: guestDefinition(),
		},
	}
}

// Register adds the resource capability to plugin.
func Register(plugin *sdk.Plugin) {
	cfg := sdkresource.ResourcePluginConfig[proxmox.Client]{
		Connections: &connectionProvider{},
		Resources:   registrations(),
		Groups: []sdkresource.ResourceGroup{{
			ID:          group,
			Name:        "Proxmox VE",
			Description: "Proxmox VE cluster resources",
			Resources: map[string][]sdkresource.ResourceMeta{
				version: {metaNode, metaStorage, metaVM, metaContainer},
			},
		}},
		DefaultDefinition: sdkresource.ResourceDefinition{IDAccessor: "id"},
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
