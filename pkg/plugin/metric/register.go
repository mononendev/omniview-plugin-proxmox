// Package metric serves node and guest metrics from the Proxmox VE RRD
// archives (/nodes/{node}/rrddata and the per-guest rrddata endpoints).
package metric

import (
	"github.com/omniviewdev/plugin-sdk/pkg/sdk"
	sdkmetric "github.com/omniviewdev/plugin-sdk/pkg/v1/metric"
)

// Resource keys served, from the resource capability.
const (
	keyNode      = "proxmox::v1::Node"
	keyVM        = "proxmox::v1::VirtualMachine"
	keyContainer = "proxmox::v1::Container"
)

// Chart groups: metrics sharing one are drawn on one chart.
const (
	groupCPU     = "CPU"
	groupMemory  = "Memory"
	groupNetwork = "Network"
	groupDiskIO  = "Disk I/O"
	groupRootFS  = "Root Filesystem"
	groupLoad    = "Load"
)

// series maps a metric to the RRD field it is read from.
type series struct {
	desc  sdkmetric.MetricDescriptor
	field string
	// scale multiplies the RRD value (cpu fractions to percent).
	scale float64
}

var shapes = []sdkmetric.MetricShape{sdkmetric.ShapeCurrent, sdkmetric.ShapeTimeseries}

func def(id, name string, unit sdkmetric.MetricUnit, icon, group, field string, scale float64) series {
	return series{
		desc: sdkmetric.MetricDescriptor{
			ID:              id,
			Name:            name,
			Unit:            unit,
			Icon:            icon,
			SupportedShapes: shapes,
			ChartGroup:      group,
		},
		field: field,
		scale: scale,
	}
}

var (
	cpu    = def("cpu", "CPU Usage", sdkmetric.UnitPercentage, "LuCpu", groupCPU, "cpu", 100)
	netIn  = def("net_in", "Network In", sdkmetric.UnitBytesPerSec, "LuArrowDownToLine", groupNetwork, "netin", 1)
	netOut = def("net_out", "Network Out", sdkmetric.UnitBytesPerSec, "LuArrowUpFromLine", groupNetwork, "netout", 1)

	nodeSeries = []series{
		cpu,
		def("iowait", "IO Wait", sdkmetric.UnitPercentage, "LuHourglass", groupCPU, "iowait", 100),
		def("memory", "Memory Used", sdkmetric.UnitBytes, "LuMemoryStick", groupMemory, "memused", 1),
		def("memory_total", "Memory Total", sdkmetric.UnitBytes, "LuMemoryStick", groupMemory, "memtotal", 1),
		netIn,
		netOut,
		def("load", "Load Average", sdkmetric.UnitCount, "LuActivity", groupLoad, "loadavg", 1),
		def("rootfs", "Root FS Used", sdkmetric.UnitBytes, "LuHardDrive", groupRootFS, "rootused", 1),
		def("rootfs_total", "Root FS Total", sdkmetric.UnitBytes, "LuHardDrive", groupRootFS, "roottotal", 1),
	}

	guestSeries = []series{
		cpu,
		def("memory", "Memory Used", sdkmetric.UnitBytes, "LuMemoryStick", groupMemory, "mem", 1),
		def("memory_total", "Memory Total", sdkmetric.UnitBytes, "LuMemoryStick", groupMemory, "maxmem", 1),
		netIn,
		netOut,
		def("disk_read", "Disk Read", sdkmetric.UnitBytesPerSec, "LuHardDriveDownload", groupDiskIO, "diskread", 1),
		def("disk_write", "Disk Write", sdkmetric.UnitBytesPerSec, "LuHardDriveUpload", groupDiskIO, "diskwrite", 1),
	}
)

// kind is how one resource key's metrics are fetched.
type kind struct {
	// guestType is "qemu" or "lxc"; empty for nodes.
	guestType string
	series    []series
}

var kinds = map[string]kind{
	keyNode:      {series: nodeSeries},
	keyVM:        {guestType: "qemu", series: guestSeries},
	keyContainer: {guestType: "lxc", series: guestSeries},
}

// Register adds the metric capability to the plugin.
func Register(plugin *sdk.Plugin) {
	handlers := make(map[string]sdkmetric.Handler, len(kinds))
	for key, k := range kinds {
		descs := make([]sdkmetric.MetricDescriptor, len(k.series))
		for i, s := range k.series {
			descs[i] = s.desc
		}
		handlers[key] = sdkmetric.Handler{Resource: key, Metrics: descs}
	}

	if err := sdkmetric.RegisterPlugin(plugin, sdkmetric.PluginOpts{
		ProviderInfo: sdkmetric.ProviderInfo{
			ID:          "proxmox",
			Name:        "Proxmox VE",
			Icon:        "SiProxmox",
			Description: "Node and guest metrics from the Proxmox VE RRD archives",
		},
		Handlers:  handlers,
		QueryFunc: query,
	}); err != nil {
		panic(err)
	}
}
