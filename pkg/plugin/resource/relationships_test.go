package resource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// TestDeclaredEdgesResolveToListedResources reads every declared extractor
// field path from the listed objects, as the host's graph indexer does, and
// checks each extracted ID is the ID of a listed resource of the target kind.
func TestDeclaredEdgesResolveToListedResources(t *testing.T) {
	byType := map[string]string{
		"node": `[{"id":"node/pve1","type":"node","node":"pve1"},{"id":"node/pve2","type":"node","node":"pve2"}]`,
		"storage": `[
			{"id":"storage/pve1/local-lvm","type":"storage","storage":"local-lvm","node":"pve1"},
			{"id":"storage/pve1/nfs","type":"storage","storage":"nfs","node":"pve1","shared":1},
			{"id":"storage/pve2/nfs","type":"storage","storage":"nfs","node":"pve2","shared":1}]`,
		"vm": `[
			{"id":"qemu/100","type":"qemu","vmid":100,"node":"pve1"},
			{"id":"qemu/101","type":"qemu","vmid":101,"node":"pve2"},
			{"id":"lxc/200","type":"lxc","vmid":200,"node":"pve1"}]`,
	}
	configs := map[string]string{
		"/api2/json/nodes/pve1/qemu/100/config": `{"scsi0":"local-lvm:vm-100-disk-0,size=32G","scsi1":"nfs:100/vm-100-disk-1.qcow2"}`,
		"/api2/json/nodes/pve2/qemu/101/config": `{"virtio0":"nfs:101/vm-101-disk-0.qcow2"}`,
		"/api2/json/nodes/pve1/lxc/200/config":  `{"rootfs":"local-lvm:subvol-200-disk-0,size=8G"}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := configs[r.URL.Path]
		if r.URL.Path == "/api2/json/cluster/resources" {
			body, ok = byType[r.URL.Query().Get("type")]
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"data":` + body + `}`))
	}))
	defer srv.Close()
	client, err := proxmox.NewClient(proxmox.Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}

	kinds := map[string]*listResourcer{
		metaNode.Key():      clusterKind(metaNode, "node", proxmox.ResourceTypeNode, "node"),
		metaStorage.Key():   clusterKind(metaStorage, "storage", proxmox.ResourceTypeStorage, "storage"),
		metaVM.Key():        clusterKind(metaVM, "virtual machine", proxmox.ResourceTypeVM, "qemu"),
		metaContainer.Key(): clusterKind(metaContainer, "container", proxmox.ResourceTypeVM, "lxc"),
	}
	objects := map[string]map[string]map[string]any{}
	for key, r := range kinds {
		list, err := r.List(context.Background(), client, sdkresource.ResourceMeta{}, sdkresource.ListInput{})
		if err != nil {
			t.Fatalf("list %s: %v", key, err)
		}
		objects[key] = map[string]map[string]any{}
		for _, raw := range list.Result {
			var obj map[string]any
			if err := json.Unmarshal(raw, &obj); err != nil {
				t.Fatal(err)
			}
			objects[key][obj["id"].(string)] = obj
		}
	}

	got := map[string][]string{}
	for key, r := range kinds {
		for _, decl := range r.DeclareRelationships() {
			for id, obj := range objects[key] {
				for _, target := range fieldPathValues(obj, decl.Extractor.FieldPath) {
					if _, ok := objects[decl.TargetResourceKey][target]; !ok {
						t.Errorf("%s %s: %s edge to %q, which is not a listed %s", key, id, decl.Type, target, decl.TargetResourceKey)
					}
					got[id] = append(got[id], string(decl.Type)+" "+target)
				}
			}
		}
	}
	for _, edges := range got {
		sort.Strings(edges)
	}

	want := map[string][]string{
		"qemu/100": {"runs_on node/pve1", "uses storage/pve1/local-lvm", "uses storage/pve1/nfs"},
		// Shared storage resolves to the entry on the guest's node.
		"qemu/101":               {"runs_on node/pve2", "uses storage/pve2/nfs"},
		"lxc/200":                {"runs_on node/pve1", "uses storage/pve1/local-lvm"},
		"storage/pve1/local-lvm": {"runs_on node/pve1"},
		"storage/pve1/nfs":       {"runs_on node/pve1"},
		"storage/pve2/nfs":       {"runs_on node/pve2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edges:\n got %v\nwant %v", got, want)
	}
}

// fieldPathValues reads a dotted path; an array yields each element.
func fieldPathValues(obj map[string]any, path string) []string {
	var cur any = obj
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	switch v := cur.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			out = append(out, e.(string))
		}
		return out
	}
	return nil
}
