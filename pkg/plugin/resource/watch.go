package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// pollInterval is how often Watch re-reads /cluster/resources.
const pollInterval = 10 * time.Second

var _ sdkresource.Watcher[proxmox.Client] = (*clusterResourcer)(nil)

// Watch polls /cluster/resources, which has no change feed, and emits the
// difference to the previous poll. A failed poll is reported as watch state
// and polling continues, so a cluster that is briefly unreachable recovers
// without exhausting the SDK's watch retries.
func (r *clusterResourcer) Watch(
	ctx context.Context,
	client *proxmox.Client,
	meta sdkresource.ResourceMeta,
	sink sdkresource.WatchEventSink,
) error {
	w := pollWatch{resourcer: r, client: client, key: meta.Key(), sink: sink, seen: map[string]json.RawMessage{}}
	sink.OnStateChange(sdkresource.WatchStateEvent{ResourceKey: w.key, State: sdkresource.WatchStateSyncing})

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		w.poll(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

type pollWatch struct {
	resourcer *clusterResourcer
	client    *proxmox.Client
	key       string
	sink      sdkresource.WatchEventSink
	// seen holds the raw entry of every ID emitted so far.
	seen map[string]json.RawMessage
	// synced is false until the first successful poll and after a failed one,
	// so the state stream carries transitions rather than one event per poll.
	synced bool
}

func (w *pollWatch) poll(ctx context.Context) {
	entries, err := w.resourcer.entries(ctx, w.client)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.synced = false
		state, code := sdkresource.WatchStateError, "PROVIDER_ERROR"
		switch {
		case proxmox.IsStatus(err, http.StatusForbidden):
			state, code = sdkresource.WatchStateForbidden, "FORBIDDEN"
		case proxmox.IsStatus(err, http.StatusUnauthorized):
			state, code = sdkresource.WatchStateForbidden, "UNAUTHORIZED"
		}
		w.sink.OnStateChange(sdkresource.WatchStateEvent{
			ResourceKey: w.key, State: state, Error: err, Message: err.Error(), ErrorCode: code,
		})
		return
	}

	current := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		current[e.ID] = struct{}{}
		meta := sdkresource.ResourceMetadata{UID: e.ID}
		prev, known := w.seen[e.ID]
		switch {
		case !known:
			w.sink.OnAdd(sdkresource.WatchAddPayload{Data: e.Raw, Key: w.key, ID: e.ID, Metadata: meta})
		case !bytes.Equal(prev, e.Raw):
			w.sink.OnUpdate(sdkresource.WatchUpdatePayload{Data: e.Raw, Key: w.key, ID: e.ID, Metadata: meta})
		}
		w.seen[e.ID] = e.Raw
	}
	for id, raw := range w.seen {
		if _, ok := current[id]; !ok {
			w.sink.OnDelete(sdkresource.WatchDeletePayload{Data: raw, Key: w.key, ID: id})
			delete(w.seen, id)
		}
	}

	if !w.synced {
		w.synced = true
		w.sink.OnStateChange(sdkresource.WatchStateEvent{
			ResourceKey: w.key, State: sdkresource.WatchStateSynced, ResourceCount: len(entries),
		})
	}
}
