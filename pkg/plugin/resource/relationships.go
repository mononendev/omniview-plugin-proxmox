package resource

import (
	"context"
	"encoding/json"
	"fmt"

	sdkresource "github.com/omniviewdev/plugin-sdk/pkg/v1/resource"

	"github.com/mononendev/omniview-plugin-proxmox/pkg/proxmox"
)

// refsField is the object the plugin adds to each entry with the IDs of the
// entries it refers to. Proxmox entries name their node and storage by bare
// name (`"node": "pve1"`); the host's relationship graph needs the target's
// resource ID (`node/pve1`), so the references are written out here and the
// relationship extractors read them.
const refsField = "omniview"

// refs is the value of refsField.
type refs struct {
	// Node is the ID of the node a guest runs on or a storage entry belongs to.
	Node string `json:"node,omitempty"`
	// Storage holds the IDs of the storage entries on the guest's node with
	// the guest's volumes (disks, EFI/TPM state, CD-ROM images, unused
	// disks): empty when it has none, absent when its config could not be
	// read.
	Storage *[]string `json:"storage,omitempty"`
}

var notNamespaced = false

var (
	relGuestNode = sdkresource.RelationshipDescriptor{
		Type:              sdkresource.RelRunsOn,
		TargetResourceKey: metaNode.Key(),
		Label:             "runs on",
		InverseLabel:      "runs",
		Cardinality:       "many-to-one",
		Extractor:         &sdkresource.RelationshipExtractor{Method: "fieldPath", FieldPath: refsField + ".node"},
		TargetNamespaced:  &notNamespaced,
	}
	relGuestStorage = sdkresource.RelationshipDescriptor{
		Type:              sdkresource.RelUses,
		TargetResourceKey: metaStorage.Key(),
		Label:             "uses storage",
		InverseLabel:      "used by",
		Cardinality:       "many-to-many",
		Extractor:         &sdkresource.RelationshipExtractor{Method: "fieldPath", FieldPath: refsField + ".storage"},
		TargetNamespaced:  &notNamespaced,
	}
	relStorageNode = sdkresource.RelationshipDescriptor{
		Type:              sdkresource.RelRunsOn,
		TargetResourceKey: metaNode.Key(),
		Label:             "attached to",
		InverseLabel:      "has storage",
		Cardinality:       "many-to-one",
		Extractor:         &sdkresource.RelationshipExtractor{Method: "fieldPath", FieldPath: refsField + ".node"},
		TargetNamespaced:  &notNamespaced,
	}
)

// relationshipsOf returns the edges the host indexes from refsField for one
// /cluster/resources type: guests run on a node and use storage entries;
// storage entries are attached to a node. Storage points at its node, like
// guests do, so a node's upstream neighbourhood holds its guests, its
// storage and the edges between them.
func relationshipsOf(entryType string) []sdkresource.RelationshipDescriptor {
	switch entryType {
	case "qemu", "lxc":
		return []sdkresource.RelationshipDescriptor{relGuestNode, relGuestStorage}
	case "storage":
		return []sdkresource.RelationshipDescriptor{relStorageNode}
	default:
		return nil
	}
}

func (r *listResourcer) DeclareRelationships() []sdkresource.RelationshipDescriptor {
	return r.relationships
}

// addRefs writes refsField into each entry of the types that declare
// relationships. A guest's storage comes from its config; a guest whose
// config cannot be read is listed without storage rather than failing the
// list.
func addRefs(ctx context.Context, client *proxmox.Client, entryType string, entries []proxmox.Entry) error {
	if entryType != "qemu" && entryType != "lxc" && entryType != "storage" {
		return nil
	}

	nodes := make([]string, len(entries))
	guests := make([]proxmox.Guest, 0, len(entries))
	for i, e := range entries {
		var head struct {
			Node string `json:"node"`
		}
		if err := json.Unmarshal(e.Raw, &head); err != nil {
			return fmt.Errorf("decode %s: %w", e.ID, err)
		}
		nodes[i] = head.Node
		if entryType != "storage" && head.Node != "" {
			guests = append(guests, proxmox.Guest{ID: e.ID, Node: head.Node})
		}
	}
	var storage map[string][]string
	if len(guests) > 0 {
		storage = client.GuestStorage(ctx, guests)
	}

	for i := range entries {
		ref := refs{}
		if nodes[i] != "" {
			ref.Node = "node/" + nodes[i]
		}
		if names, ok := storage[entries[i].ID]; ok {
			ids := make([]string, len(names))
			for j, name := range names {
				ids[j] = "storage/" + nodes[i] + "/" + name
			}
			ref.Storage = &ids
		}
		raw, err := withField(entries[i].Raw, refsField, ref)
		if err != nil {
			return fmt.Errorf("encode %s: %w", entries[i].ID, err)
		}
		entries[i].Raw = raw
	}
	return nil
}

// withField returns obj with field set to value. Keys come out sorted, so
// equal objects encode to equal bytes and the watch diff stays exact.
func withField(obj json.RawMessage, field string, value any) (json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(obj, &m); err != nil {
		return nil, err
	}
	v, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	m[field] = v
	return json.Marshal(m)
}
