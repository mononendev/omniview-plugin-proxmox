import { RESOURCE_KEYS } from '../constants';

/**
 * Field the backend adds to guest and storage entries with the IDs of the
 * entries they refer to; the host's relationship graph is built from it.
 */
export const REFS_FIELD = 'omniview';

/** Value of REFS_FIELD. `storage` is absent when the backend could not list storage. */
export type Refs = { node?: string; storage?: string[] };

export function refsOf(obj: Record<string, unknown>): Refs {
  const refs = obj[REFS_FIELD];
  return refs && typeof refs === 'object' ? (refs as Refs) : {};
}

/** Splits a storage entry ID, `storage/{node}/{storage}`. */
export function parseStorageId(id: string): { node: string; storage: string } {
  const [, node = '', ...rest] = id.split('/');
  return { node, storage: rest.join('/') };
}

/** Resource kind of a Proxmox cluster-unique ID (`qemu/100`, `node/pve1`, ...). */
export function resourceKeyOf(id: string): string | undefined {
  switch (id.split('/')[0]) {
    case 'node':
      return RESOURCE_KEYS.node;
    case 'storage':
      return RESOURCE_KEYS.storage;
    case 'qemu':
      return RESOURCE_KEYS.virtualMachine;
    case 'lxc':
      return RESOURCE_KEYS.container;
    default:
      return undefined;
  }
}
