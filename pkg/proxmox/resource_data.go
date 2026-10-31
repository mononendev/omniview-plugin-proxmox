package proxmox

import (
	resourcev2 "github.com/omniviewdev/plugin-sdk/pkg/resource/v2"
)

// ResourceData returns the resource body a shell, log or metric request
// carries as native values, with integers as int64 or uint64. An absent or
// invalid body, or one that is not an object, yields nil.
func ResourceData(body *resourcev2.Value) map[string]any {
	if body == nil {
		return nil
	}
	native, err := resourcev2.NativeFromValue(*body)
	if err != nil {
		return nil
	}
	data, _ := native.(map[string]any)
	return data
}
