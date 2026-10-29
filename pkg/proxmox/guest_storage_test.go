package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestGuestStorageReadsVolumesFromConfigs(t *testing.T) {
	qemu100 := `{"scsi0":"local-lvm:vm-100-disk-0,iothread=1,size=32G","efidisk0":"local-lvm:vm-100-disk-1,size=4M",` +
		`"ide2":"local:iso/debian.iso,media=cdrom","ide0":"none,media=cdrom","unused0":"nfs:100/vm-100-disk-2.qcow2",` +
		`"scsi1":"/dev/disk/by-id/ata-DISK,size=1T","net0":"virtio=BC:24:11:00:00:01,bridge=vmbr0",` +
		`"description":"backup:weekly","cores":4}`
	configs := map[string]string{
		"/api2/json/nodes/pve1/qemu/100/config": qemu100,
		"/api2/json/nodes/pve2/qemu/100/config": qemu100,
		"/api2/json/nodes/pve2/lxc/200/config": `{"rootfs":"local-zfs:subvol-200-disk-0,size=8G",` +
			`"mp0":"/mnt/host,mp=/data","mp1":"tank:subvol-200-disk-1,mp=/srv,size=100G"}`,
	}
	var (
		mu       sync.Mutex
		requests []string
		failing  = map[string]bool{"/api2/json/nodes/pve1/qemu/101/config": true}
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, r.URL.Path)
		body, ok := configs[r.URL.Path]
		if !ok || failing[r.URL.Path] {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"data":` + body + `}`))
	}))
	defer srv.Close()
	client, err := NewClient(Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	takeRequests := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := requests
		requests = nil
		return out
	}

	guests := []Guest{{ID: "qemu/100", Node: "pve1"}, {ID: "qemu/101", Node: "pve1"}, {ID: "lxc/200", Node: "pve2"}}
	want := map[string][]string{
		// Disks, EFI vars, ISO and unused disks; not the empty drive,
		// passed-through device, NIC or a description that looks like a volume.
		"qemu/100": {"local", "local-lvm", "nfs"},
		// Bind mounts have no storage. qemu/101's config cannot be read.
		"lxc/200": {"local-zfs", "tank"},
	}
	if got := client.GuestStorage(ctx, guests); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	takeRequests()

	// Within the TTL only the guest without a result is read again.
	client.GuestStorage(ctx, guests)
	if got := takeRequests(); !reflect.DeepEqual(got, []string{"/api2/json/nodes/pve1/qemu/101/config"}) {
		t.Fatalf("cached read requested %q", got)
	}

	// A migrated guest is read from its new node at once.
	guests[0].Node = "pve2"
	if got := client.GuestStorage(ctx, guests); !reflect.DeepEqual(got, want) {
		t.Fatalf("after migration got %v, want %v", got, want)
	}
	if got := takeRequests(); !reflect.DeepEqual(got, []string{"/api2/json/nodes/pve1/qemu/101/config", "/api2/json/nodes/pve2/qemu/100/config"}) &&
		!reflect.DeepEqual(got, []string{"/api2/json/nodes/pve2/qemu/100/config", "/api2/json/nodes/pve1/qemu/101/config"}) {
		t.Fatalf("after migration requested %q", got)
	}

	// After the TTL a failed read keeps the previous result.
	for id, cached := range client.storage.guests {
		cached.readAt = time.Now().Add(-guestConfigTTL)
		client.storage.guests[id] = cached
	}
	mu.Lock()
	failing["/api2/json/nodes/pve2/qemu/100/config"] = true
	mu.Unlock()
	if got := client.GuestStorage(ctx, guests); !reflect.DeepEqual(got, want) {
		t.Fatalf("after failed refresh got %v, want %v", got, want)
	}
}
