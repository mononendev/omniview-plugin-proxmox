package resource

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	resourcev2 "github.com/omniviewdev/plugin-sdk/pkg/resource/v2"

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

// guestAction is one lifecycle action: POST .../status/{id}.
type guestAction struct {
	descriptor resourcev2.ActionDescriptor
	// done is the past-tense verb of the result message: "Started".
	done string
	// qemuOnly actions are not offered for containers.
	qemuOnly bool
}

var guestActions = []guestAction{
	{done: "Started", descriptor: resourcev2.ActionDescriptor{
		ID: "start", Label: "Start", Icon: "LuPlay",
		Description: "Start the guest.",
	}},
	{done: "Shut down", descriptor: resourcev2.ActionDescriptor{
		ID: "shutdown", Label: "Shutdown", Icon: "LuPower",
		Description: "Shut the guest down cleanly (ACPI or the guest agent for VMs, the init system for containers).",
	}},
	{done: "Rebooted", descriptor: resourcev2.ActionDescriptor{
		ID: "reboot", Label: "Reboot", Icon: "LuRotateCw",
		Description: "Reboot the guest cleanly.",
	}},
	{done: "Stopped", descriptor: resourcev2.ActionDescriptor{
		ID: "stop", Label: "Stop", Icon: "LuSquare", Dangerous: true,
		Description: "Stop the guest immediately, like pulling the power plug. Unsaved guest data is lost.",
	}},
	{done: "Suspended", qemuOnly: true, descriptor: resourcev2.ActionDescriptor{
		ID: "suspend", Label: "Pause", Icon: "LuPause",
		Description: "Pause the virtual machine's execution in memory.",
	}},
	{done: "Resumed", qemuOnly: true, descriptor: resourcev2.ActionDescriptor{
		ID: "resume", Label: "Resume", Icon: "LuPlay",
		Description: "Resume a paused virtual machine.",
	}},
}

const actionConfig = "config"

var configAction = resourcev2.ActionDescriptor{
	ID: actionConfig, Label: "Configuration", Icon: "LuFileText",
	Description: "Read the guest's configuration and its digest.",
}

// actions lists the lifecycle actions of this guest kind.
func (k *kind) actions() []guestAction {
	out := make([]guestAction, 0, len(guestActions))
	for _, a := range guestActions {
		if a.qemuOnly && k.guestType != "qemu" {
			continue
		}
		a.descriptor.Scope = resourcev2.ActionScopeInstance
		out = append(out, a)
	}
	return out
}

// actionDescriptors declares the lifecycle actions plus the config read.
func (k *kind) actionDescriptors() []resourcev2.ActionDescriptor {
	actions := k.actions()
	out := make([]resourcev2.ActionDescriptor, 0, len(actions)+1)
	for _, a := range actions {
		out = append(out, a.descriptor)
	}
	cfg := configAction
	cfg.Scope = resourcev2.ActionScopeInstance
	return append(out, cfg)
}

// action runs a guest action. Lifecycle actions return {message, upid};
// config returns {config, digest, node}.
func (k *kind) action(ctx context.Context, client *proxmox.Client, request resourcev2.ActionRequest) (resourcev2.Value, error) {
	if request.TypeTarget != nil {
		return resourcev2.Value{}, resourcev2.NewError("UNSUPPORTED", "Guest actions act on one guest, not on a type.")
	}
	id := request.Reference.Address.ID
	if request.Action == actionConfig {
		return k.readConfig(ctx, client, id)
	}
	for _, a := range k.actions() {
		if a.descriptor.ID == request.Action {
			return k.lifecycle(ctx, client, id, a)
		}
	}
	return resourcev2.Value{}, &resourcev2.ResourceOperationError{
		Code:    "UNSUPPORTED",
		Title:   "Unsupported action",
		Message: "Action " + request.Action + " is not available for a " + k.noun + ".",
	}
}

// lifecycle runs a status action and waits, bounded, for its task.
func (k *kind) lifecycle(ctx context.Context, client *proxmox.Client, id string, a guestAction) (resourcev2.Value, error) {
	what := a.descriptor.ID + " " + k.noun + " " + id
	g, err := k.find(ctx, client, id)
	if err != nil {
		return resourcev2.Value{}, err
	}
	upid, err := client.GuestStatusAction(ctx, g, a.descriptor.ID)
	if err != nil {
		return resourcev2.Value{}, &writeError{privilege: privPowerMgmt, what: what, err: err}
	}
	done, err := waitTask(ctx, client, g.Node, upid)
	if err != nil {
		return resourcev2.Value{}, &writeError{privilege: privPowerMgmt, what: what, err: err}
	}
	message := a.descriptor.Label + " of " + g.Label() + " is still running."
	if done {
		message = a.done + " " + g.Label() + "."
	}
	return resourcev2.ValueFromNative(map[string]any{"message": message, "upid": upid})
}

// readConfig is the read-only config action.
func (k *kind) readConfig(ctx context.Context, client *proxmox.Client, id string) (resourcev2.Value, error) {
	g, err := k.find(ctx, client, id)
	if err != nil {
		return resourcev2.Value{}, err
	}
	config, digest, err := client.GuestConfig(ctx, g)
	if err != nil {
		return resourcev2.Value{}, &writeError{privilege: privAudit, what: "read the configuration of " + k.noun + " " + id, err: err}
	}
	return resourcev2.ValueFromNative(map[string]any{"config": config, "digest": digest, "node": g.Node})
}

// mutate applies a config update or deletes the guest.
func (k *kind) mutate(ctx context.Context, client *proxmox.Client, request resourcev2.MutationRequest) (resourcev2.Record, error) {
	switch request.Operation {
	case "update":
		if request.ExpectedRevision != nil {
			return resourcev2.Record{}, resourcev2.NewError("INVALID_REQUEST",
				"Guest updates take the configuration digest in their input, not an expected revision.")
		}
		return k.update(ctx, client, request)
	case "delete":
		return k.delete(ctx, client, request.Reference)
	default:
		return resourcev2.Record{}, resourcev2.NewError("UNSUPPORTED", "Guests can only be updated or deleted.")
	}
}

// update applies the input, an object of config keys to values, to the
// guest's config and returns the refreshed guest. A null or "" value deletes
// the key; "digest" is passed through to reject concurrent changes.
func (k *kind) update(ctx context.Context, client *proxmox.Client, request resourcev2.MutationRequest) (resourcev2.Record, error) {
	set, del, digest, err := configChanges(request.Input)
	if err != nil {
		return resourcev2.Record{}, err
	}
	id := request.Reference.Address.ID
	g, err := k.find(ctx, client, id)
	if err != nil {
		return resourcev2.Record{}, err
	}
	if len(set) > 0 || len(del) > 0 {
		if err := client.UpdateGuestConfig(ctx, g, set, del, digest); err != nil {
			return resourcev2.Record{}, &writeError{privilege: privConfig, what: "change the configuration of " + k.noun + " " + id, err: err}
		}
	}
	return k.get(ctx, client, resourcev2.GetRequest{
		Route: request.Route, Reference: request.Reference, Representation: representationID,
	})
}

// delete destroys the guest, its unreferenced disks and its backup,
// replication and HA entries, and returns its last listing. Proxmox refuses
// a running guest.
func (k *kind) delete(ctx context.Context, client *proxmox.Client, ref resourcev2.ResourceReference) (resourcev2.Record, error) {
	id := ref.Address.ID
	what := "delete " + k.noun + " " + id
	g, err := k.find(ctx, client, id)
	if err != nil {
		return resourcev2.Record{}, err
	}
	upid, err := client.DeleteGuest(ctx, g)
	if err != nil {
		return resourcev2.Record{}, &writeError{privilege: privAllocate, what: what, err: err}
	}
	if _, err := waitTask(ctx, client, g.Node, upid); err != nil {
		return resourcev2.Record{}, &writeError{privilege: privAllocate, what: what, err: err}
	}
	return record(ref, proxmox.Entry{ID: g.ID, Raw: g.Raw})
}

// find locates a guest of this kind on its current node.
func (k *kind) find(ctx context.Context, client *proxmox.Client, id string) (proxmox.GuestRef, error) {
	if !strings.HasPrefix(id, k.guestType+"/") {
		return proxmox.GuestRef{}, &proxmox.GuestNotFoundError{ID: id}
	}
	return client.FindGuest(ctx, id)
}

// configChanges splits an update input into the keys to set (as Proxmox
// form values), the keys to delete and the digest.
func configChanges(input resourcev2.Value) (map[string]string, []string, string, error) {
	invalid := func(msg string) error {
		return &resourcev2.ResourceOperationError{Code: "INVALID_INPUT", Title: "Invalid configuration", Message: msg}
	}
	native, err := resourcev2.NativeFromValue(input)
	values, isMap := native.(map[string]any)
	if err != nil || !isMap {
		return nil, nil, "", invalid("The update must be an object of configuration keys to values.")
	}
	set := map[string]string{}
	var del []string
	var digest string
	for key, value := range values {
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
		case int64:
			set[key] = strconv.FormatInt(v, 10)
		case uint64:
			set[key] = strconv.FormatUint(v, 10)
		case float64:
			set[key] = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			set[key] = map[bool]string{true: "1", false: "0"}[v]
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
