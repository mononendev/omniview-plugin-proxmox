package resource

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// lister returns every native entry of one kind.
type lister func(context.Context, *proxmox.Client) ([]proxmox.Entry, error)

// listResourcer serves one read-only kind from a lister. Proxmox has no
// cheaper per-object read for most kinds, so Get and Watch are built on the
// full list. Objects are the native entries, plus a refsField object on
// guests and storage, identified by the lister's ID: Proxmox's own
// cluster-unique `id` for /cluster/resources kinds (qemu/100, node/pve1,
// storage/pve1/local), the Ceph name otherwise.
type listResourcer struct {
	// noun names one object in messages: "virtual machine", "Ceph pool".
	noun   string
	list   lister
	policy sdkresource.SyncPolicy
	// relationships are the edges the host graph extracts from the objects.
	relationships []sdkresource.RelationshipDescriptor
}

var (
	_ sdkresource.Resourcer[proxmox.Client] = (*listResourcer)(nil)
	_ sdkresource.ErrorClassifier           = (*listResourcer)(nil)
	_ sdkresource.SyncPolicyDeclarer        = (*listResourcer)(nil)
	_ sdkresource.RelationshipDeclarer      = (*listResourcer)(nil)
)

// clusterKind serves the /cluster/resources entries of one type, watched
// from connect, with refsField added where the type declares relationships.
func clusterKind(noun string, apiType proxmox.ResourceType, entryType string) *listResourcer {
	return &listResourcer{
		noun:          noun,
		policy:        sdkresource.SyncOnConnect,
		relationships: relationshipsOf(entryType),
		list: func(ctx context.Context, client *proxmox.Client) ([]proxmox.Entry, error) {
			all, err := client.ClusterResources(ctx, apiType)
			if err != nil {
				return nil, err
			}
			out := make([]proxmox.Entry, 0, len(all))
			for _, e := range all {
				if e.Type == entryType {
					out = append(out, e.Entry)
				}
			}
			if err := addRefs(ctx, client, entryType, out); err != nil {
				return nil, err
			}
			return out, nil
		},
	}
}

// cephKind serves one Ceph kind from a Client method expression, e.g.
// (*proxmox.Client).CephPools. It is watched only once opened: most clusters
// have no Ceph, and those that do need no Ceph polling until asked.
func cephKind(noun string, method func(*proxmox.Client, context.Context) ([]proxmox.Entry, error)) *listResourcer {
	return &listResourcer{
		noun:   noun,
		policy: sdkresource.SyncOnFirstQuery,
		list: func(ctx context.Context, client *proxmox.Client) ([]proxmox.Entry, error) {
			return method(client, ctx)
		},
	}
}

func (r *listResourcer) SyncPolicy() sdkresource.SyncPolicy { return r.policy }

func (r *listResourcer) List(
	ctx context.Context,
	client *proxmox.Client,
	_ sdkresource.ResourceMeta,
	_ sdkresource.ListInput,
) (*sdkresource.ListResult, error) {
	entries, err := r.list(ctx, client)
	if err != nil {
		return nil, err
	}
	result := make([]json.RawMessage, len(entries))
	for i, e := range entries {
		result[i] = e.Raw
	}
	return &sdkresource.ListResult{Result: result, Success: true, TotalCount: len(result)}, nil
}

func (r *listResourcer) Get(
	ctx context.Context,
	client *proxmox.Client,
	_ sdkresource.ResourceMeta,
	input sdkresource.GetInput,
) (*sdkresource.GetResult, error) {
	entries, err := r.list(ctx, client)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.ID == input.ID {
			return &sdkresource.GetResult{Result: e.Raw, Success: true}, nil
		}
	}
	return nil, &sdkresource.ResourceOperationError{
		Code:    "NOT_FOUND",
		Title:   "Not found",
		Message: "No " + r.noun + " with ID " + input.ID + " exists in this cluster.",
	}
}

func (r *listResourcer) Find(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta, sdkresource.FindInput,
) (*sdkresource.FindResult, error) {
	return nil, unsupported("Find")
}

func (r *listResourcer) Create(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta, sdkresource.CreateInput,
) (*sdkresource.CreateResult, error) {
	return nil, unsupported("Create")
}

func (r *listResourcer) Update(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta, sdkresource.UpdateInput,
) (*sdkresource.UpdateResult, error) {
	return nil, unsupported("Update")
}

func (r *listResourcer) Delete(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta, sdkresource.DeleteInput,
) (*sdkresource.DeleteResult, error) {
	return nil, unsupported("Delete")
}

// ClassifyError maps Proxmox API failures to structured errors.
func (r *listResourcer) ClassifyError(err error) error {
	var opErr *sdkresource.ResourceOperationError
	if errors.As(err, &opErr) {
		return err
	}
	var cephErr *proxmox.CephUnavailableError
	switch {
	case proxmox.IsStatus(err, http.StatusUnauthorized):
		return &sdkresource.ResourceOperationError{
			Err: err, Code: "UNAUTHORIZED", Title: "Unauthorized",
			Message: "The Proxmox API token was rejected.",
		}
	case proxmox.IsStatus(err, http.StatusForbidden):
		return &sdkresource.ResourceOperationError{
			Err: err, Code: "FORBIDDEN", Title: "Forbidden",
			Message:     "The Proxmox API token lacks permission to read " + r.noun + " objects.",
			Suggestions: []string{"Grant the token PVEAuditor on /."},
		}
	case errors.As(err, &cephErr):
		return &sdkresource.ResourceOperationError{
			Err: err, Code: "CEPH_UNAVAILABLE", Title: "Ceph unavailable", Message: err.Error(),
			Suggestions: []string{
				"Ceph must be installed and initialised (pveceph install, pveceph init) on an online node, with its monitors in quorum.",
				"A cluster without Ceph can ignore the Ceph section.",
			},
		}
	default:
		return &sdkresource.ResourceOperationError{
			Err: err, Code: "PROVIDER_ERROR", Title: "Proxmox API error", Message: err.Error(),
		}
	}
}

func unsupported(op string) error {
	return &sdkresource.ResourceOperationError{
		Code:    "UNSUPPORTED",
		Title:   "Unsupported operation",
		Message: op + " is not supported by the Proxmox plugin yet.",
	}
}
