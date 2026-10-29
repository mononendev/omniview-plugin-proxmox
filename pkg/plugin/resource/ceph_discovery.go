package resource

import (
	"context"
	"errors"

	"github.com/omniviewdev/plugin-sdk/pkg/types"
	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// cephDiscovery gives a connection its kinds: every static kind, plus the
// Ceph kinds when its cluster runs Ceph. The SDK runs it on every connection
// start, so Ceph installed later shows up on the next connect.
//
// The static kinds must be listed too: the SDK treats a non-empty discovered
// set as the connection's complete list and skips the connect-time watches
// of every kind outside it.
//
// A failed probe yields no kinds at all, so the SDK watches every static kind
// and offers no Ceph kinds: it drops discovery errors, and a cluster that
// cannot answer `ceph status` has nothing to show under them.
type cephDiscovery struct {
	static []sdkresource.ResourceMeta
	ceph   []sdkresource.ResourceMeta
}

var _ sdkresource.DiscoveryProvider = (*cephDiscovery)(nil)

func (d *cephDiscovery) Discover(ctx context.Context, conn *types.Connection) ([]sdkresource.ResourceMeta, error) {
	client, err := newClient(conn)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return d.discover(ctx, client)
}

func (d *cephDiscovery) discover(ctx context.Context, client *proxmox.Client) ([]sdkresource.ResourceMeta, error) {
	out := make([]sdkresource.ResourceMeta, 0, len(d.static)+len(d.ceph))
	out = append(out, d.static...)
	if _, err := client.CephStatus(ctx); err != nil {
		var unavailable *proxmox.CephUnavailableError
		if errors.As(err, &unavailable) {
			return out, nil
		}
		return nil, err
	}
	return append(out, d.ceph...), nil
}

func (d *cephDiscovery) OnConnectionRemoved(context.Context, *types.Connection) error { return nil }
