package resource

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

func TestLoadConnectionsKeepsSecretOutOfConnectionData(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "clusters.yaml")
	body := `
clusters:
  - id: lab
    url: https://pve.lan:8006
    token_id: root@pam!omni
    token_secret: s3cret
`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	conns, err := loadConnections([]string{filepath.Join(dir, "missing.yaml"), file})
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 || conns[0].ID != "lab" || conns[0].Name != "lab" {
		t.Fatalf("unexpected connections: %+v", conns)
	}
	for k, v := range conns[0].Data {
		if v == "s3cret" {
			t.Fatalf("token secret exposed in Connection.Data[%q]", k)
		}
	}

	if _, err := loadConnections([]string{file, file}); err == nil {
		t.Fatal("expected duplicate cluster id across files to be rejected")
	}
}

func TestGuestResourcersAreExactPerKind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"qemu/100","type":"qemu","vmid":100,"node":"pve1"},` +
			`{"id":"lxc/101","type":"lxc","vmid":101,"node":"pve1"}]}`))
	}))
	defer srv.Close()
	client, err := proxmox.NewClient(proxmox.Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	vms := clusterKind("virtual machine", proxmox.ResourceTypeVM, "qemu")

	list, err := vms.List(ctx, client, metaVM, sdkresource.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if list.TotalCount != 1 || len(list.Result) != 1 {
		t.Fatalf("VM list should contain only qemu guests, got %d", len(list.Result))
	}

	if _, err := vms.Get(ctx, client, metaVM, sdkresource.GetInput{ID: "qemu/100"}); err != nil {
		t.Fatalf("get qemu/100: %v", err)
	}
	_, err = vms.Get(ctx, client, metaVM, sdkresource.GetInput{ID: "lxc/101"})
	var opErr *sdkresource.ResourceOperationError
	if !errors.As(err, &opErr) || opErr.Code != "NOT_FOUND" {
		t.Fatalf("VM Get must not resolve a container ID, got %v", err)
	}
}

func TestCephKindOnClusterWithoutCephIsCephUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/cluster/resources" {
			_, _ = w.Write([]byte(`{"data":[{"id":"node/pve1","type":"node","node":"pve1","status":"online"}]}`))
			return
		}
		http.Error(w, "binary not installed: /usr/bin/ceph-mon", http.StatusInternalServerError)
	}))
	defer srv.Close()
	client, err := proxmox.NewClient(proxmox.Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	pools := cephKind("Ceph pool", (*proxmox.Client).CephPools)

	_, err = pools.List(context.Background(), client, metaCephPool, sdkresource.ListInput{})
	var opErr *sdkresource.ResourceOperationError
	if !errors.As(pools.ClassifyError(err), &opErr) || opErr.Code != "CEPH_UNAVAILABLE" {
		t.Fatalf("want CEPH_UNAVAILABLE, got %v", pools.ClassifyError(err))
	}
}
