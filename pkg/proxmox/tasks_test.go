package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A token's UPID holds "user@realm!tokenid". Its escaped segment must reach
// Proxmox escaped once: escaped twice, "!" arrives as a literal "%21" and
// Proxmox answers "no such task" for a task that exists.
func TestWaitTaskAddressesATokenUPIDExactly(t *testing.T) {
	const upid = "UPID:pve1:0006062F:1C8ABE39:6AE2CDA2:qmdestroy:100:omniview@pve!omniview:"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve1/tasks/"+upid+"/status" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"data":null,"errors":{"upid":"no such task"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"status":"stopped","exitstatus":"OK"}}`))
	}))
	defer srv.Close()
	client, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "omniview@pve!omniview", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.WaitTask(context.Background(), "pve1", upid); err != nil {
		t.Fatalf("WaitTask: %v", err)
	}
}
