// Package exec registers the Proxmox shell capability: node shells and guest
// consoles through PVE's termproxy.
package exec

import (
	"errors"
	"fmt"

	"github.com/omniviewdev/plugin-sdk/pkg/sdk"
	"github.com/omniviewdev/plugin-sdk/pkg/types"
	sdkexec "github.com/omniviewdev/plugin-sdk/pkg/v1/exec"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

const (
	pluginID     = "proxmox"
	keyNode      = "proxmox::v1::Node"
	keyContainer = "proxmox::v1::Container"
	keyVM        = "proxmox::v1::VirtualMachine"

	// ParamSerial selects the serial interface (serial0..serial3) of a VM
	// console. Defaults to serial0.
	ParamSerial = "serial"
)

// Register adds the exec capability to plugin.
func Register(plugin *sdk.Plugin) {
	if err := sdkexec.RegisterPlugin(plugin, sdkexec.PluginOpts{Handlers: handlers(connectionClient)}); err != nil {
		panic(fmt.Sprintf("register proxmox exec capability: %v", err))
	}
}

// clientFunc returns the API client for a session's connection; tests inject
// one pointing at a fake server.
type clientFunc func(ctx *types.PluginContext) (*proxmox.Client, error)

func connectionClient(ctx *types.PluginContext) (*proxmox.Client, error) {
	if ctx == nil {
		return nil, errors.New("no plugin context")
	}
	return proxmox.ClientForConnection(ctx.Connection)
}

// handlers lists one handler per shell-capable kind. DefaultCommand only
// labels the session: PVE decides what runs (login for nodes, the container
// console, `qm terminal` on a VM serial port).
func handlers(clients clientFunc) []sdkexec.Handler {
	return []sdkexec.Handler{
		{
			Plugin:         pluginID,
			Resource:       keyNode,
			DefaultCommand: []string{"login"},
			TTYHandler:     shellHandler(clients, nodeTarget),
			HandlesResize:  true,
		},
		{
			Plugin:         pluginID,
			Resource:       keyContainer,
			DefaultCommand: []string{"lxc-console"},
			TTYHandler:     shellHandler(clients, guestTarget(guestLXC)),
			HandlesResize:  true,
		},
		{
			Plugin:         pluginID,
			Resource:       keyVM,
			DefaultCommand: []string{"qm", "terminal"},
			TTYHandler:     shellHandler(clients, guestTarget(guestQemu)),
			HandlesResize:  true,
		},
	}
}
