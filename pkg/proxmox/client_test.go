package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clusters.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFileRejectsInvalidClusters(t *testing.T) {
	cases := map[string]string{
		"duplicate id": `
clusters:
  - {id: a, url: "https://pve:8006", token_id: "root@pam!t", token_secret: s}
  - {id: a, url: "https://pve2:8006", token_id: "root@pam!t", token_secret: s}`,
		"url with api path": `
clusters:
  - {id: a, url: "https://pve:8006/api2/json", token_id: "root@pam!t", token_secret: s}`,
		"token id without token name": `
clusters:
  - {id: a, url: "https://pve:8006", token_id: "root@pam", token_secret: s}`,
		"unknown field": `
clusters:
  - {id: a, url: "https://pve:8006", token_id: "root@pam!t", token_secret: s, password: x}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadFile(writeConfig(t, body)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestClientSendsTokenAndReturnsNativeEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken=root@pam!omni=secret" {
			http.Error(w, "authentication failure", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api2/json/cluster/resources" || r.URL.Query().Get("type") != "vm" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"qemu/100","type":"qemu","vmid":100,"node":"pve1","extra":{"k":1}},` +
			`{"id":"lxc/101","type":"lxc","vmid":101,"node":"pve2"}]}`))
	}))
	defer srv.Close()

	client, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!omni", TokenSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.ClusterResources(context.Background(), ResourceTypeVM)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "qemu/100" || got[1].Type != "lxc" {
		t.Fatalf("unexpected entries: %+v", got)
	}
	if !strings.Contains(string(got[0].Raw), `"extra":{"k":1}`) {
		t.Fatalf("native fields were not preserved: %s", got[0].Raw)
	}

	bad, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!omni", TokenSecret: "wrong"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = bad.Version(context.Background())
	if !IsStatus(err, http.StatusUnauthorized) {
		t.Fatalf("expected 401 APIError, got %v", err)
	}
	if strings.Contains(err.Error(), "wrong") {
		t.Fatalf("error leaked the token secret: %v", err)
	}
}
