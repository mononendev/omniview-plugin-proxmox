package resource

import (
	"context"
	"errors"

	"github.com/omniviewdev/plugin-sdk/pkg/types"
	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// cephDiscovery gives a connection the Ceph kinds when its cluster runs Ceph.
// The SDK runs it on every connection start, so Ceph installed later shows up
// on the next connect. A failed probe yields no Ceph kinds: the SDK drops
// discovery errors, and a cluster that cannot answer `ceph status` has
// nothing to show under them.
type cephDiscovery struct {
	metas []sdkresource.ResourceMeta
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
	if _, err := client.CephStatus(ctx); err != nil {
		var unavailable *proxmox.CephUnavailableError
		if errors.As(err, &unavailable) {
			return nil, nil
		}
		return nil, err
	}
	return d.metas, nil
}

func (d *cephDiscovery) OnConnectionRemoved(context.Context, *types.Connection) error { return nil }
