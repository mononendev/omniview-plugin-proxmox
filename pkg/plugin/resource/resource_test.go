package resource

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
    console_user: omniview@pve
    console_password: c0nsole-pw
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
		if s, _ := v.(string); strings.Contains(s, "s3cret") || strings.Contains(s, "c0nsole-pw") {
			t.Fatalf("secret exposed in Connection.Data[%q]", k)
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
	vms := clusterKind(metaVM, "virtual machine", proxmox.ResourceTypeVM, "qemu")

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
	pools := cephKind(metaCephPool, "Ceph pool", (*proxmox.Client).CephPools)

	_, err = pools.List(context.Background(), client, metaCephPool, sdkresource.ListInput{})
	var opErr *sdkresource.ResourceOperationError
	if !errors.As(pools.ClassifyError(err), &opErr) || opErr.Code != "UNAVAILABLE" || opErr.Title != "Ceph unavailable" {
		t.Fatalf("want Ceph unavailable, got %v", pools.ClassifyError(err))
	}
}

// The SDK skips the connect-time watch of every static kind missing from a
// non-empty discovered set, so discovery must always list them; otherwise a
// Ceph cluster gets no Node/Storage/guest watches and an empty graph.
func TestDiscoveryKeepsStaticKindsAndAddsCephOnlyWhereCephAnswers(t *testing.T) {
	static := []sdkresource.ResourceMeta{metaNode, metaStorage, metaVM, metaContainer}
	ceph := []sdkresource.ResourceMeta{metaCephCluster, metaCephPool}
	for _, tc := range []struct {
		name string
		ceph bool
	}{
		{name: "no ceph", ceph: false},
		{name: "ceph", ceph: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api2/json/cluster/resources":
					_, _ = w.Write([]byte(`{"data":[{"id":"node/pve1","type":"node","node":"pve1","status":"online"}]}`))
				case tc.ceph && r.URL.Path == "/api2/json/nodes/pve1/ceph/status":
					_, _ = w.Write([]byte(`{"data":{"fsid":"f1","health":{"status":"HEALTH_OK"}}}`))
				default:
					http.Error(w, "binary not installed: /usr/bin/ceph-mon", http.StatusInternalServerError)
				}
			}))
			defer srv.Close()
			client, err := proxmox.NewClient(proxmox.Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
			if err != nil {
				t.Fatal(err)
			}

			got, err := (&cephDiscovery{static: static, ceph: ceph}).discover(context.Background(), client)
			if err != nil {
				t.Fatal(err)
			}
			keys := map[string]bool{}
			for _, m := range got {
				keys[m.Key()] = true
			}
			for _, m := range static {
				if !keys[m.Key()] {
					t.Errorf("static kind %s missing from discovery: its watch would be skipped", m.Key())
				}
			}
			for _, m := range ceph {
				if keys[m.Key()] != tc.ceph {
					t.Errorf("Ceph kind %s discovered=%v, want %v", m.Key(), keys[m.Key()], tc.ceph)
				}
			}
		})
	}
}
