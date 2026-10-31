package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	resourcev2 "github.com/omniviewdev/plugin-sdk/pkg/resource/v2"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// pollInterval is how often Watch re-reads a kind.
const pollInterval = 10 * time.Second

// watch polls the kind, which has no change feed in Proxmox, and emits the
// difference to the previous poll. A failed poll is reported as watch state
// and polling continues, so a cluster that is briefly unreachable recovers
// without exhausting the SDK's watch retries.
func (k *kind) watch(
	ctx context.Context,
	client *proxmox.Client,
	target resourcev2.ReadTarget,
	_ resourcev2.Representation,
	sink resourcev2.WatchEventSink,
) error {
	w := pollWatch{kind: k, client: client, target: target, sink: sink, seen: map[string]seenEntry{}}
	w.state(resourcev2.WatchStateSyncing, nil, 0)

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
	kind   *kind
	client *proxmox.Client
	target resourcev2.ReadTarget
	sink   resourcev2.WatchEventSink
	// seen holds every entry emitted so far, by ID.
	seen map[string]seenEntry
	// synced is false until the first successful poll and after a failed one,
	// so the state stream carries transitions rather than one event per poll.
	synced bool
}

type seenEntry struct {
	raw    json.RawMessage
	record resourcev2.Record
}

func (w *pollWatch) poll(ctx context.Context) {
	entries, err := w.kind.list(ctx, w.client)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.synced = false
		state := resourcev2.WatchStateError
		if proxmox.IsStatus(err, http.StatusForbidden) || proxmox.IsStatus(err, http.StatusUnauthorized) {
			state = resourcev2.WatchStateForbidden
		}
		w.state(state, err, 0)
		return
	}

	route := w.target.Route
	guarantee := resourcev2.BestEffortGuarantee
	current := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		current[e.ID] = struct{}{}
		prev, known := w.seen[e.ID]
		if known && bytes.Equal(prev.raw, e.Raw) {
			continue
		}
		rec, err := record(reference(w.target.Provider, w.target.Authority, w.target.Type, e.ID), e)
		if err != nil {
			// One unreadable object must not stop the others; it stays unseen.
			delete(current, e.ID)
			continue
		}
		w.seen[e.ID] = seenEntry{raw: e.Raw, record: rec}
		if known {
			w.sink.OnUpdate(resourcev2.WatchUpdatePayload{Route: route, Record: rec, Guarantee: guarantee})
		} else {
			w.sink.OnAdd(resourcev2.WatchAddPayload{Route: route, Record: rec, Guarantee: guarantee})
		}
	}
	for id, prev := range w.seen {
		if _, ok := current[id]; !ok {
			last := prev.record
			w.sink.OnDelete(resourcev2.WatchDeletePayload{
				Route: route, Reference: last.Reference, LastKnown: &last, Guarantee: guarantee,
			})
			delete(w.seen, id)
		}
	}

	if !w.synced {
		w.synced = true
		w.state(resourcev2.WatchStateSynced, nil, len(w.seen))
	}
}

// state reports a watch state; err, when set, is classified for display.
func (w *pollWatch) state(state resourcev2.WatchState, err error, count int) {
	event := resourcev2.WatchStateEvent{
		Route:     w.target.Route,
		Type:      w.target.Type,
		State:     state,
		Count:     resourcev2.Count{Kind: "unknown"},
		Guarantee: resourcev2.BestEffortGuarantee,
	}
	if state == resourcev2.WatchStateSynced {
		n := strconv.Itoa(count)
		event.Count = resourcev2.Count{Kind: "exact", Value: &n}
	}
	if err != nil {
		var opErr *resourcev2.ResourceOperationError
		if errors.As(w.kind.ClassifyError(err), &opErr) {
			event.Error = opErr
			event.Message = opErr.Message
		}
	}
	w.sink.OnStateChange(event)
}
