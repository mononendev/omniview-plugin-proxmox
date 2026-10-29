package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// taskWaitBound is how long a guest write waits for its task. A task still
// running then (a slow shutdown) is reported as started, not failed.
var taskWaitBound = 60 * time.Second

// Privileges a guest write needs, named in FORBIDDEN errors.
const (
	privPowerMgmt = "VM.PowerMgmt"
	privAllocate  = "VM.Allocate"
	privAudit     = "VM.Audit"
	privConfig    = "VM.Config.*"
)

// guestResourcer serves a guest kind (qemu or lxc): the cluster listing plus
// lifecycle and config actions, config updates and deletion. Only guest
// kinds are guestResourcers, so only they advertise actions.
type guestResourcer struct {
	*listResourcer
	// guestType is the /cluster/resources type and ID prefix: qemu or lxc.
	guestType string
}

var (
	_ sdkresource.Resourcer[proxmox.Client]       = (*guestResourcer)(nil)
	_ sdkresource.ActionResourcer[proxmox.Client] = (*guestResourcer)(nil)
	_ sdkresource.ErrorClassifier                 = (*guestResourcer)(nil)
	_ sdkresource.SyncPolicyDeclarer              = (*guestResourcer)(nil)
	_ sdkresource.RelationshipDeclarer            = (*guestResourcer)(nil)
)

// guestKind serves the guests of guestType (qemu or lxc).
func guestKind(meta sdkresource.ResourceMeta, noun, guestType string) *guestResourcer {
	return &guestResourcer{
		listResourcer: clusterKind(meta, noun, proxmox.ResourceTypeVM, guestType),
		guestType:     guestType,
	}
}

// guestAction is one lifecycle action: POST .../status/{id}.
type guestAction struct {
	descriptor sdkresource.ActionDescriptor
	// done is the past-tense verb of the result message: "Started".
	done string
	// qemuOnly actions are not offered for containers.
	qemuOnly bool
}

var guestActions = []guestAction{
	{done: "Started", descriptor: sdkresource.ActionDescriptor{
		ID: "start", Label: "Start", Icon: "LuPlay",
		Description: "Start the guest.",
	}},
	{done: "Shut down", descriptor: sdkresource.ActionDescriptor{
		ID: "shutdown", Label: "Shutdown", Icon: "LuPower",
		Description: "Shut the guest down cleanly (ACPI or the guest agent for VMs, the init system for containers).",
	}},
	{done: "Rebooted", descriptor: sdkresource.ActionDescriptor{
		ID: "reboot", Label: "Reboot", Icon: "LuRotateCw",
		Description: "Reboot the guest cleanly.",
	}},
	{done: "Stopped", descriptor: sdkresource.ActionDescriptor{
		ID: "stop", Label: "Stop", Icon: "LuSquare", Dangerous: true,
		Description: "Stop the guest immediately, like pulling the power plug. Unsaved guest data is lost.",
	}},
	{done: "Suspended", qemuOnly: true, descriptor: sdkresource.ActionDescriptor{
		ID: "suspend", Label: "Pause", Icon: "LuPause",
		Description: "Pause the virtual machine's execution in memory.",
	}},
	{done: "Resumed", qemuOnly: true, descriptor: sdkresource.ActionDescriptor{
		ID: "resume", Label: "Resume", Icon: "LuPlay",
		Description: "Resume a paused virtual machine.",
	}},
}

const actionConfig = "config"

var configAction = sdkresource.ActionDescriptor{
	ID: actionConfig, Label: "Configuration", Icon: "LuFileText",
	Description: "Read the guest's configuration and its digest.",
}

func (r *guestResourcer) actions() []guestAction {
	out := make([]guestAction, 0, len(guestActions))
	for _, a := range guestActions {
		if a.qemuOnly && r.guestType != "qemu" {
			continue
		}
		a.descriptor.Scope = sdkresource.ActionScopeInstance
		out = append(out, a)
	}
	return out
}

func (r *guestResourcer) GetActions(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta,
) ([]sdkresource.ActionDescriptor, error) {
	actions := r.actions()
	out := make([]sdkresource.ActionDescriptor, 0, len(actions)+1)
	for _, a := range actions {
		out = append(out, a.descriptor)
	}
	cfg := configAction
	cfg.Scope = sdkresource.ActionScopeInstance
	return append(out, cfg), nil
}

func (r *guestResourcer) ExecuteAction(
	ctx context.Context,
	client *proxmox.Client,
	_ sdkresource.ResourceMeta,
	actionID string,
	input sdkresource.ActionInput,
) (*sdkresource.ActionResult, error) {
	if actionID == actionConfig {
		return r.readConfig(ctx, client, input.ID)
	}
	for _, a := range r.actions() {
		if a.descriptor.ID == actionID {
			return r.lifecycle(ctx, client, input.ID, a)
		}
	}
	return nil, &sdkresource.ResourceOperationError{
		Code:    "UNSUPPORTED",
		Title:   "Unsupported action",
		Message: "Action " + actionID + " is not available for a " + r.noun + ".",
	}
}

func (r *guestResourcer) StreamAction(
	context.Context, *proxmox.Client, sdkresource.ResourceMeta, string, sdkresource.ActionInput, chan<- sdkresource.ActionEvent,
) error {
	return unsupported("Streaming actions")
}

// lifecycle runs a status action and waits, bounded, for its task.
func (r *guestResourcer) lifecycle(
	ctx context.Context, client *proxmox.Client, id string, a guestAction,
) (*sdkresource.ActionResult, error) {
	what := a.descriptor.ID + " " + r.noun + " " + id
	g, err := r.find(ctx, client, id)
	if err != nil {
		return nil, err
	}
	upid, err := client.GuestStatusAction(ctx, g, a.descriptor.ID)
	if err != nil {
		return nil, &writeError{privilege: privPowerMgmt, what: what, err: err}
	}
	done, err := waitTask(ctx, client, g.Node, upid)
	if err != nil {
		return nil, &writeError{privilege: privPowerMgmt, what: what, err: err}
	}
	result := &sdkresource.ActionResult{Success: true, Data: map[string]any{"upid": upid}}
	if done {
		result.Message = a.done + " " + g.Label() + "."
	} else {
		result.Message = a.descriptor.Label + " of " + g.Label() + " is still running."
	}
	return result, nil
}

// readConfig is the read-only config action.
func (r *guestResourcer) readConfig(ctx context.Context, client *proxmox.Client, id string) (*sdkresource.ActionResult, error) {
	g, err := r.find(ctx, client, id)
	if err != nil {
		return nil, err
	}
	config, digest, err := client.GuestConfig(ctx, g)
	if err != nil {
		return nil, &writeError{privilege: privAudit, what: "read the configuration of " + r.noun + " " + id, err: err}
	}
	return &sdkresource.ActionResult{
		Success: true,
		Data:    map[string]any{"config": config, "digest": digest, "node": g.Node},
	}, nil
}

// Update applies UpdateInput.Input, a JSON object of config keys to values,
// to the guest's config and returns the refreshed guest. A null or "" value
// deletes the key; "digest" is passed through to reject concurrent changes.
func (r *guestResourcer) Update(
	ctx context.Context,
	client *proxmox.Client,
	meta sdkresource.ResourceMeta,
	input sdkresource.UpdateInput,
) (*sdkresource.UpdateResult, error) {
	set, del, digest, err := configChanges(input.Input)
	if err != nil {
		return nil, err
	}
	g, err := r.find(ctx, client, input.ID)
	if err != nil {
		return nil, err
	}
	if len(set) > 0 || len(del) > 0 {
		if err := client.UpdateGuestConfig(ctx, g, set, del, digest); err != nil {
			return nil, &writeError{privilege: privConfig, what: "change the configuration of " + r.noun + " " + input.ID, err: err}
		}
	}
	got, err := r.Get(ctx, client, meta, sdkresource.GetInput{ID: input.ID})
	if err != nil {
		return nil, err
	}
	return &sdkresource.UpdateResult{Result: got.Result, Success: true}, nil
}

// Delete destroys the guest, its unreferenced disks and its backup,
// replication and HA entries. Proxmox refuses a running guest.
func (r *guestResourcer) Delete(
	ctx context.Context,
	client *proxmox.Client,
	_ sdkresource.ResourceMeta,
	input sdkresource.DeleteInput,
) (*sdkresource.DeleteResult, error) {
	what := "delete " + r.noun + " " + input.ID
	g, err := r.find(ctx, client, input.ID)
	if err != nil {
		return nil, err
	}
	upid, err := client.DeleteGuest(ctx, g)
	if err != nil {
		return nil, &writeError{privilege: privAllocate, what: what, err: err}
	}
	if _, err := waitTask(ctx, client, g.Node, upid); err != nil {
		return nil, &writeError{privilege: privAllocate, what: what, err: err}
	}
	return &sdkresource.DeleteResult{Result: g.Raw, Success: true}, nil
}

// find locates a guest of this kind on its current node.
func (r *guestResourcer) find(ctx context.Context, client *proxmox.Client, id string) (proxmox.GuestRef, error) {
	if !strings.HasPrefix(id, r.guestType+"/") {
		return proxmox.GuestRef{}, &proxmox.GuestNotFoundError{ID: id}
	}
	return client.FindGuest(ctx, id)
}

// configChanges splits an Update input into the keys to set (as Proxmox
// form values), the keys to delete and the digest.
func configChanges(raw json.RawMessage) (map[string]string, []string, string, error) {
	invalid := func(msg string) error {
		return &sdkresource.ResourceOperationError{Code: "INVALID", Title: "Invalid configuration", Message: msg}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var input map[string]any
	if err := dec.Decode(&input); err != nil || input == nil {
		return nil, nil, "", invalid("The update must be a JSON object of configuration keys to values.")
	}
	set := map[string]string{}
	var del []string
	var digest string
	for key, value := range input {
		if key == "digest" {
			s, ok := value.(string)
			if !ok && value != nil {
				return nil, nil, "", invalid("digest must be a string.")
			}
			digest = s
			continue
		}
		if key == "" || key == "delete" {
			return nil, nil, "", invalid("Invalid configuration key " + `"` + key + `".`)
		}
		switch v := value.(type) {
		case nil:
			del = append(del, key)
		case string:
			if v == "" {
				del = append(del, key)
			} else {
				set[key] = v
			}
		case json.Number:
			set[key] = v.String()
		case bool:
			if v {
				set[key] = "1"
			} else {
				set[key] = "0"
			}
		default:
			return nil, nil, "", invalid("The value of " + key + " must be a string, number, boolean or null.")
		}
	}
	return set, del, digest, nil
}

// waitTask waits for a guest task for at most taskWaitBound. done is false
// when the bound expired with the task still running.
func waitTask(ctx context.Context, client *proxmox.Client, node, upid string) (done bool, err error) {
	waitCtx, cancel := context.WithTimeout(ctx, taskWaitBound)
	defer cancel()
	err = client.WaitTask(waitCtx, node, upid)
	if err != nil && ctx.Err() == nil && errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
		return false, nil
	}
	return err == nil, err
}

// writeError is a failed guest write: what it tried and the privilege it
// needs, for ClassifyError.
type writeError struct {
	privilege string
	// what is the attempted operation: "start virtual machine qemu/100".
	what string
	err  error
}

func (e *writeError) Error() string { return e.what + ": " + e.err.Error() }
func (e *writeError) Unwrap() error { return e.err }
