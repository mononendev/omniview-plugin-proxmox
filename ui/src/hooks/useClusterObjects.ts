import { useMemo } from 'react';
import { useResources } from '@omniviewdev/runtime';

import { PLUGIN_ID, RESOURCE_KEYS } from '../constants';
import type { ProxmoxObject } from '../utils/columns';

export type ClusterObject = { resourceKey: string; data: ProxmoxObject };

/**
 * Every node, storage entry and guest of a connection: per kind, and by
 * Proxmox ID. Graph nodes carry only IDs; `objects` supplies names and status
 * for drawing them.
 */
export function useClusterObjects(connectionId: string) {
  const opts = { pluginID: PLUGIN_ID, connectionID: connectionId, idAccessor: 'id' };
  const nodes = useResources({ ...opts, resourceKey: RESOURCE_KEYS.node }).resources;
  const storage = useResources({ ...opts, resourceKey: RESOURCE_KEYS.storage }).resources;
  const vms = useResources({ ...opts, resourceKey: RESOURCE_KEYS.virtualMachine }).resources;
  const containers = useResources({ ...opts, resourceKey: RESOURCE_KEYS.container }).resources;

  const objects = useMemo(() => {
    const byId = new Map<string, ClusterObject>();
    const add = (resourceKey: string, data: { result?: unknown[] } | null | undefined) => {
      for (const obj of (data?.result ?? []) as ProxmoxObject[]) byId.set(obj.id, { resourceKey, data: obj });
    };
    add(RESOURCE_KEYS.node, nodes.data);
    add(RESOURCE_KEYS.storage, storage.data);
    add(RESOURCE_KEYS.virtualMachine, vms.data);
    add(RESOURCE_KEYS.container, containers.data);
    return byId;
  }, [nodes.data, storage.data, vms.data, containers.data]);

  return {
    objects,
    nodes: (nodes.data?.result ?? []) as ProxmoxObject[],
    storage: (storage.data?.result ?? []) as ProxmoxObject[],
    virtualMachines: (vms.data?.result ?? []) as ProxmoxObject[],
    containers: (containers.data?.result ?? []) as ProxmoxObject[],
    isLoading: nodes.isLoading || storage.isLoading || vms.isLoading || containers.isLoading,
    error: nodes.error ?? storage.error ?? vms.error ?? containers.error,
  };
}
