package resource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

type recordingSink struct{ events []string }

func (s *recordingSink) OnAdd(p sdkresource.WatchAddPayload) {
	s.events = append(s.events, "add "+p.ID)
}
func (s *recordingSink) OnUpdate(p sdkresource.WatchUpdatePayload) {
	s.events = append(s.events, "update "+p.ID)
}
func (s *recordingSink) OnDelete(p sdkresource.WatchDeletePayload) {
	s.events = append(s.events, "delete "+p.ID)
}
func (s *recordingSink) OnStateChange(e sdkresource.WatchStateEvent) {
	s.events = append(s.events, "state "+map[sdkresource.WatchState]string{
		sdkresource.WatchStateSynced:    "synced",
		sdkresource.WatchStateError:     "error",
		sdkresource.WatchStateForbidden: "forbidden",
	}[e.State])
}

func TestPollEmitsChangesAndStateTransitions(t *testing.T) {
	// Each poll serves the next response; a nil body is a 500.
	responses := [][]map[string]any{
		{{"id": "qemu/100", "type": "qemu", "status": "running"}, {"id": "qemu/101", "type": "qemu", "status": "stopped"}},
		{{"id": "qemu/100", "type": "qemu", "status": "running"}, {"id": "qemu/101", "type": "qemu", "status": "stopped"}},
		nil,
		{{"id": "qemu/100", "type": "qemu", "status": "stopped"}, {"id": "lxc/200", "type": "lxc"}, {"id": "qemu/102", "type": "qemu"}},
	}
	var call atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := responses[call.Add(1)-1]
		if body == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
	}))
	defer srv.Close()
	client, err := proxmox.NewClient(proxmox.Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}

	sink := &recordingSink{}
	w := pollWatch{
		resourcer: clusterKind("virtual machine", proxmox.ResourceTypeVM, "qemu"),
		client:    client,
		key:       metaVM.Key(),
		sink:      sink,
		seen:      map[string]json.RawMessage{},
	}
	var got [][]string
	for range responses {
		sink.events = nil
		w.poll(context.Background())
		got = append(got, sink.events)
	}

	want := [][]string{
		{"add qemu/100", "add qemu/101", "state synced"},
		nil, // unchanged poll: no events, no repeated state
		{"state error"},
		{"update qemu/100", "add qemu/102", "delete qemu/101", "state synced"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events per poll:\n got %q\nwant %q", got, want)
	}
}
