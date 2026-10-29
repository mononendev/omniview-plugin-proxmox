package resource

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// fakePVE answers the guest list, guest configs and task status, and
// records write requests. Writes answer write (status, body).
type fakePVE struct {
	t          *testing.T
	mu         sync.Mutex
	writes     []recorded
	writeCode  int
	writeBody  string
	taskStatus string // "stopped" or "running"
	exitStatus string
}

type recorded struct {
	method string
	path   string
	query  url.Values
	form   url.Values
}

func newFakePVE(t *testing.T) (*fakePVE, *proxmox.Client) {
	t.Helper()
	f := &fakePVE{t: t, writeCode: http.StatusOK, writeBody: `{"data":"UPID:pve2:00001:task"}`, taskStatus: "stopped", exitStatus: "OK"}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	client, err := proxmox.NewClient(proxmox.Cluster{ID: "c", URL: srv.URL, TokenID: "root@pam!t", TokenSecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	return f, client
}

func (f *fakePVE) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/api2/json")
	switch {
	case r.Method == http.MethodGet && path == "/cluster/resources":
		_, _ = w.Write([]byte(`{"data":[{"id":"qemu/100","type":"qemu","vmid":100,"node":"pve2","name":"web01","status":"running"},` +
			`{"id":"lxc/200","type":"lxc","vmid":200,"node":"pve1","name":"ct","status":"stopped"}]}`))
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/config"):
		_, _ = w.Write([]byte(`{"data":{"cores":2,"scsi0":"local-lvm:vm-100-disk-0,size=8G","digest":"d1"}}`))
	case r.Method == http.MethodGet && strings.Contains(path, "/tasks/"):
		if path != "/nodes/pve2/tasks/UPID:pve2:00001:task/status" {
			f.t.Errorf("task polled at %s", path)
		}
		_, _ = w.Write([]byte(`{"data":{"status":"` + f.taskStatus + `","exitstatus":"` + f.exitStatus + `"}}`))
	default:
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		f.writes = append(f.writes, recorded{method: r.Method, path: path, query: r.URL.Query(), form: form})
		w.WriteHeader(f.writeCode)
		_, _ = w.Write([]byte(f.writeBody))
	}
}

var ctx = context.Background()

func opError(t *testing.T, r *guestResourcer, err error) *sdkresource.ResourceOperationError {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	var opErr *sdkresource.ResourceOperationError
	if !errors.As(r.ClassifyError(err), &opErr) {
		t.Fatalf("not classified: %v", err)
	}
	return opErr
}

func TestOnlyGuestKindsHaveActions(t *testing.T) {
	var node sdkresource.Resourcer[proxmox.Client] = clusterKind(metaNode, "node", proxmox.ResourceTypeNode, "node")
	if _, ok := node.(sdkresource.ActionResourcer[proxmox.Client]); ok {
		t.Fatal("node kind must not advertise actions")
	}

	ids := func(r *guestResourcer) []string {
		actions, err := r.GetActions(ctx, nil, sdkresource.ResourceMeta{})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, a := range actions {
			if a.Scope != sdkresource.ActionScopeInstance {
				t.Fatalf("%s: scope %q", a.ID, a.Scope)
			}
			if a.Dangerous != (a.ID == "stop") {
				t.Fatalf("%s: dangerous %v", a.ID, a.Dangerous)
			}
			out = append(out, a.ID)
		}
		return out
	}
	vm := strings.Join(ids(guestKind(metaVM, "virtual machine", "qemu")), ",")
	ct := strings.Join(ids(guestKind(metaContainer, "container", "lxc")), ",")
	if vm != "start,shutdown,reboot,stop,suspend,resume,config" || ct != "start,shutdown,reboot,stop,config" {
		t.Fatalf("vm actions %s, container actions %s", vm, ct)
	}
}

func TestLifecycleActionPostsAndWaitsForTask(t *testing.T) {
	f, client := newFakePVE(t)
	vms := guestKind(metaVM, "virtual machine", "qemu")

	res, err := vms.ExecuteAction(ctx, client, metaVM, "start", sdkresource.ActionInput{ID: "qemu/100"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Message != "Started web01 (qemu/100)." {
		t.Fatalf("unexpected result %+v", res)
	}
	if len(f.writes) != 1 || f.writes[0].method != http.MethodPost || f.writes[0].path != "/nodes/pve2/qemu/100/status/start" {
		t.Fatalf("unexpected writes %+v", f.writes)
	}

	f.exitStatus = "can't lock file '/var/lock/qemu-server/lock-100.conf' - got timeout"
	_, err = vms.ExecuteAction(ctx, client, metaVM, "stop", sdkresource.ActionInput{ID: "qemu/100"})
	opErr := opError(t, vms, err)
	if opErr.Code != "TASK_FAILED" || !strings.Contains(opErr.Message, "got timeout") {
		t.Fatalf("unexpected error %+v", opErr)
	}
	if f.writes[1].path != "/nodes/pve2/qemu/100/status/stop" {
		t.Fatalf("stop posted to %s", f.writes[1].path)
	}
}

func TestLifecycleActionStillRunningAfterBound(t *testing.T) {
	f, client := newFakePVE(t)
	f.taskStatus = "running"
	f.exitStatus = ""
	defer func(d time.Duration) { taskWaitBound = d }(taskWaitBound)
	taskWaitBound = 50 * time.Millisecond

	vms := guestKind(metaVM, "virtual machine", "qemu")
	res, err := vms.ExecuteAction(ctx, client, metaVM, "shutdown", sdkresource.ActionInput{ID: "qemu/100"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || !strings.Contains(res.Message, "still running") {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestActionRejectsOtherKindsAndUnknownActions(t *testing.T) {
	_, client := newFakePVE(t)
	cts := guestKind(metaContainer, "container", "lxc")

	_, err := cts.ExecuteAction(ctx, client, metaContainer, "start", sdkresource.ActionInput{ID: "qemu/100"})
	if opErr := opError(t, cts, err); opErr.Code != "NOT_FOUND" {
		t.Fatalf("container resourcer acted on a VM: %+v", opErr)
	}
	_, err = cts.ExecuteAction(ctx, client, metaContainer, "suspend", sdkresource.ActionInput{ID: "lxc/200"})
	if opErr := opError(t, cts, err); opErr.Code != "UNSUPPORTED" {
		t.Fatalf("container suspend: %+v", opErr)
	}
}

func TestConfigActionReturnsConfigDigestAndNode(t *testing.T) {
	_, client := newFakePVE(t)
	vms := guestKind(metaVM, "virtual machine", "qemu")

	res, err := vms.ExecuteAction(ctx, client, metaVM, "config", sdkresource.ActionInput{ID: "qemu/100"})
	if err != nil {
		t.Fatal(err)
	}
	config, _ := res.Data["config"].(map[string]any)
	if !res.Success || res.Message != "" || res.Data["digest"] != "d1" || res.Data["node"] != "pve2" || config["cores"] != float64(2) {
		t.Fatalf("unexpected result %+v", res)
	}
	if _, ok := config["digest"]; ok {
		t.Fatal("digest left in config")
	}
}

func TestDeleteDestroysGuestAndWaits(t *testing.T) {
	f, client := newFakePVE(t)
	vms := guestKind(metaVM, "virtual machine", "qemu")

	res, err := vms.Delete(ctx, client, metaVM, sdkresource.DeleteInput{ID: "qemu/100"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || !strings.Contains(string(res.Result), `"qemu/100"`) {
		t.Fatalf("unexpected result %+v", res)
	}
	w := f.writes[0]
	if w.method != http.MethodDelete || w.path != "/nodes/pve2/qemu/100" ||
		w.query.Get("purge") != "1" || w.query.Get("destroy-unreferenced-disks") != "1" {
		t.Fatalf("unexpected delete %+v", w)
	}

	if _, err := clusterKind(metaNode, "node", proxmox.ResourceTypeNode, "node").Delete(ctx, client, metaNode, sdkresource.DeleteInput{ID: "node/pve1"}); err == nil {
		t.Fatal("node delete must stay unsupported")
	}
}

func TestUpdateEncodesChangesAndReturnsGuest(t *testing.T) {
	f, client := newFakePVE(t)
	f.writeBody = `{"data":null}`
	cts := guestKind(metaContainer, "container", "lxc")

	input := json.RawMessage(`{"cores":4,"memory":2048.5,"onboot":true,"protection":false,"hostname":"ct2",` +
		`"description":"","tags":null,"digest":"d1"}`)
	res, err := cts.Update(ctx, client, metaContainer, sdkresource.UpdateInput{ID: "lxc/200", Input: input})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || !strings.Contains(string(res.Result), `"id":"lxc/200"`) || !strings.Contains(string(res.Result), refsField) {
		t.Fatalf("unexpected result %s", res.Result)
	}
	w := f.writes[0]
	want := url.Values{
		"cores": {"4"}, "memory": {"2048.5"}, "onboot": {"1"}, "protection": {"0"}, "hostname": {"ct2"},
		"delete": {"description,tags"}, "digest": {"d1"},
	}
	if w.method != http.MethodPut || w.path != "/nodes/pve1/lxc/200/config" || w.form.Encode() != want.Encode() {
		t.Fatalf("unexpected update %s %s %v", w.method, w.path, w.form)
	}

	_, err = cts.Update(ctx, client, metaContainer, sdkresource.UpdateInput{ID: "lxc/200", Input: json.RawMessage(`{"net0":{"x":1}}`)})
	if opErr := opError(t, cts, err); opErr.Code != "INVALID" {
		t.Fatalf("nested value: %+v", opErr)
	}
}

func TestWriteErrorsAreClassified(t *testing.T) {
	f, client := newFakePVE(t)
	vms := guestKind(metaVM, "virtual machine", "qemu")

	f.writeCode, f.writeBody = http.StatusForbidden, `{"data":null}`
	_, err := vms.ExecuteAction(ctx, client, metaVM, "reboot", sdkresource.ActionInput{ID: "qemu/100"})
	opErr := opError(t, vms, err)
	if opErr.Code != "FORBIDDEN" || !strings.Contains(opErr.Message, "VM.PowerMgmt") ||
		len(opErr.Suggestions) != 1 || !strings.Contains(opErr.Suggestions[0], "PVEVMAdmin") {
		t.Fatalf("reboot 403: %+v", opErr)
	}
	_, err = vms.Delete(ctx, client, metaVM, sdkresource.DeleteInput{ID: "qemu/100"})
	if opErr := opError(t, vms, err); opErr.Code != "FORBIDDEN" || !strings.Contains(opErr.Message, "VM.Allocate") {
		t.Fatalf("delete 403: %+v", opErr)
	}
	_, err = vms.Update(ctx, client, metaVM, sdkresource.UpdateInput{ID: "qemu/100", Input: json.RawMessage(`{"cores":2}`)})
	if opErr := opError(t, vms, err); opErr.Code != "FORBIDDEN" || !strings.Contains(opErr.Message, "VM.Config.") {
		t.Fatalf("update 403: %+v", opErr)
	}

	f.writeCode = http.StatusBadRequest
	f.writeBody = `{"data":null,"errors":{"cores":"value must have a minimum value of 1","memory":"invalid format"}}`
	_, err = vms.Update(ctx, client, metaVM, sdkresource.UpdateInput{ID: "qemu/100", Input: json.RawMessage(`{"cores":0}`)})
	opErr = opError(t, vms, err)
	if opErr.Code != "INVALID" || !strings.Contains(opErr.Message, "cores: value must have a minimum value of 1; memory: invalid format") {
		t.Fatalf("update 400: %+v", opErr)
	}

	// A read 403 keeps the read message.
	if opErr := opError(t, vms, &proxmox.APIError{StatusCode: http.StatusForbidden}); !strings.Contains(opErr.Message, "read") {
		t.Fatalf("read 403: %+v", opErr)
	}
}
