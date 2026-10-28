package resource

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// clusterResourcer serves one kind from GET /cluster/resources. Objects are
// the unmodified native entries and are identified by Proxmox's own
// cluster-unique `id` (qemu/100, node/pve1, storage/pve1/local), so Get is
// exact without inventing a namespace.
type clusterResourcer struct {
	apiType   proxmox.ResourceType
	entryType string
}

var (
	_ sdkresource.Resourcer[proxmox.Client] = (*clusterResourcer)(nil)
	_ sdkresource.ErrorClassifier           = (*clusterResourcer)(nil)
)

func (r *clusterResourcer) entries(ctx context.Context, client *proxmox.Client) ([]proxmox.ClusterResource, error) {
	all, err := client.ClusterResources(ctx, r.apiType)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, e := range all {
		if e.Type == r.entryType {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *clusterResourcer) List(
	ctx context.Context,
	client *proxmox.Client,
	_ sdkresource.ResourceMeta,
	_ sdkresource.ListInput,
) (*sdkresource.ListResult, error) {
	entries, err := r.entries(ctx, client)
	if err != nil {
		return nil, err
	}
	result := make([]json.RawMessage, len(entries))
	for i, e := range entries {
		result[i] = e.Raw
	}
	return &sdkresource.ListResult{Result: result, Success: true, TotalCount: len(result)}, nil
}

func (r *clusterResourcer) Get(
	ctx context.Context,
	client *proxmox.Client,
	_ sdkresource.ResourceMeta,
	input sdkresource.GetInput,
) (*sdkresource.GetResult, error) {
	entries, err := r.entries(ctx, client)
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
		Message: "No " + r.entryType + " with ID " + input.ID + " exists in this cluster.",
	}
}

func (r *clusterResourcer) Find(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta, sdkresource.FindInput,
) (*sdkresource.FindResult, error) {
	return nil, unsupported("Find")
}

func (r *clusterResourcer) Create(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta, sdkresource.CreateInput,
) (*sdkresource.CreateResult, error) {
	return nil, unsupported("Create")
}

func (r *clusterResourcer) Update(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta, sdkresource.UpdateInput,
) (*sdkresource.UpdateResult, error) {
	return nil, unsupported("Update")
}

func (r *clusterResourcer) Delete(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta, sdkresource.DeleteInput,
) (*sdkresource.DeleteResult, error) {
	return nil, unsupported("Delete")
}

// ClassifyError maps Proxmox API failures to structured errors.
func (r *clusterResourcer) ClassifyError(err error) error {
	var opErr *sdkresource.ResourceOperationError
	if errors.As(err, &opErr) {
		return err
	}
	switch {
	case proxmox.IsStatus(err, http.StatusUnauthorized):
		return &sdkresource.ResourceOperationError{
			Err: err, Code: "UNAUTHORIZED", Title: "Unauthorized",
			Message: "The Proxmox API token was rejected.",
		}
	case proxmox.IsStatus(err, http.StatusForbidden):
		return &sdkresource.ResourceOperationError{
			Err: err, Code: "FORBIDDEN", Title: "Forbidden",
			Message:     "The Proxmox API token lacks permission to read " + r.entryType + " resources.",
			Suggestions: []string{"Grant the token PVEAuditor on /."},
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
