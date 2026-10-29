package proxmox

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func entryIDs(entries []Entry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

func TestCephRequestsFallBackToANodeRunningCeph(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []string
		pve2Up   atomic.Bool
	)
	pve2Up.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api2/json")
		mu.Lock()
		requests = append(requests, path)
		mu.Unlock()
		switch {
		case path == "/cluster/resources":
			_, _ = w.Write([]byte(`{"data":[{"id":"node/pve2","type":"node","node":"pve2","status":"online"},` +
				`{"id":"node/pve3","type":"node","node":"pve3","status":"offline"},` +
				`{"id":"node/pve1","type":"node","node":"pve1","status":"online"}]}`))
		case path == "/nodes/pve2/ceph/pool" && pve2Up.Load():
			_, _ = w.Write([]byte(`{"data":[{"pool":1,"pool_name":".mgr","size":3},{"pool":2,"pool_name":"rbd","size":3}]}`))
		default:
			http.Error(w, "binary not installed", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	client, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	call := func() ([]Entry, []string, error) {
		mu.Lock()
		requests = nil
		mu.Unlock()
		got, err := client.CephPools(ctx)
		mu.Lock()
		defer mu.Unlock()
		return got, requests, err
	}

	got, reqs, err := call()
	if err != nil {
		t.Fatal(err)
	}
	if ids := entryIDs(got); !reflect.DeepEqual(ids, []string{".mgr", "rbd"}) {
		t.Fatalf("pool IDs = %q", ids)
	}
	// Online nodes in name order; the offline pve3 is never asked.
	if want := []string{"/cluster/resources", "/nodes/pve1/ceph/pool", "/nodes/pve2/ceph/pool"}; !reflect.DeepEqual(reqs, want) {
		t.Fatalf("first call requests:\n got %q\nwant %q", reqs, want)
	}

	if _, reqs, err = call(); err != nil || !reflect.DeepEqual(reqs, []string{"/nodes/pve2/ceph/pool"}) {
		t.Fatalf("second call should go straight to pve2, got requests %q err %v", reqs, err)
	}

	pve2Up.Store(false)
	_, reqs, err = call()
	var cephErr *CephUnavailableError
	if !errors.As(err, &cephErr) || len(cephErr.Errs) != 2 {
		t.Fatalf("expected CephUnavailableError with one error per online node, got %v", err)
	}
	if want := []string{"/nodes/pve2/ceph/pool", "/cluster/resources", "/nodes/pve1/ceph/pool"}; !reflect.DeepEqual(reqs, want) {
		t.Fatalf("failing call requests:\n got %q\nwant %q", reqs, want)
	}
}

func TestCephRequestsStopAtPermissionErrors(t *testing.T) {
	var cephCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cluster/resources") {
			_, _ = w.Write([]byte(`{"data":[{"node":"pve1","status":"online"},{"node":"pve2","status":"online"}]}`))
			return
		}
		cephCalls.Add(1)
		http.Error(w, "Permission check failed", http.StatusForbidden)
	}))
	defer srv.Close()
	client, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CephMonitors(context.Background())
	if !IsStatus(err, http.StatusForbidden) || cephCalls.Load() != 1 {
		t.Fatalf("a 403 applies to every node: want it returned after one call, got %v after %d", err, cephCalls.Load())
	}
}

func TestCephOSDsAreTheLeavesOfTheCrushTree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cluster/resources") {
			_, _ = w.Write([]byte(`{"data":[{"node":"pve1","status":"online"}]}`))
			return
		}
		// Abridged /nodes/{node}/ceph/osd: a root with a rack holding a host,
		// a second root reusing osd.1, and flags beside the tree.
		_, _ = w.Write([]byte(`{"data":{"flags":"noout","root":{"leaf":0,"children":[
			{"id":-1,"name":"default","type":"root","children":[
				{"id":-5,"name":"rack1","type":"rack","children":[
					{"id":-3,"name":"pve1","type":"host","children":[
						{"id":0,"name":"osd.0","type":"osd","host":"pve1","status":"up","in":1,"leaf":1},
						{"id":1,"name":"osd.1","type":"osd","host":"pve1","status":"down","in":0,"leaf":1}]}]}]},
			{"id":-9,"name":"ssd","type":"root","children":[
				{"id":1,"name":"osd.1","type":"osd","status":"down","leaf":1},
				{"id":2,"name":"osd.2","type":"osd","status":"up","leaf":1}]}]}}}`))
	}))
	defer srv.Close()
	client, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.CephOSDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ids := entryIDs(got); !reflect.DeepEqual(ids, []string{"osd.0", "osd.1", "osd.2"}) {
		t.Fatalf("OSD IDs = %q", ids)
	}
	if !strings.Contains(string(got[1].Raw), `"host":"pve1"`) {
		t.Fatalf("osd.1 should keep the entry from its host bucket: %s", got[1].Raw)
	}
}
