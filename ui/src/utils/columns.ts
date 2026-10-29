import type { StatusDotStatus } from '@omniviewdev/base-ui';

import { RESOURCE_KEYS } from '../constants';

/** A Proxmox `/cluster/resources` entry, passed through unmodified by the backend. */
export type ProxmoxObject = Record<string, unknown> & { id: string };

/**
 * One table column over a top-level field of a `/cluster/resources` entry.
 * Columns live here rather than in the backend's ResourceDefinition: the
 * resource transport has no RPC that delivers definitions to the UI.
 */
export type Column = {
  field: string;
  header: string;
  /** Byte count, rendered as KiB/MiB/… and right-aligned. */
  bytes?: boolean;
  /** Rendered as a status dot coloured by STATUS_BY_VALUE. */
  status?: boolean;
};

const statusColumn: Column = { field: 'status', header: 'Status', status: true };

const guestColumns: Column[] = [
  { field: 'vmid', header: 'VMID' },
  { field: 'name', header: 'Name' },
  { field: 'node', header: 'Node' },
  statusColumn,
  { field: 'maxcpu', header: 'vCPUs' },
  { field: 'mem', header: 'Memory', bytes: true },
  { field: 'maxmem', header: 'Max memory', bytes: true },
  { field: 'maxdisk', header: 'Disk', bytes: true },
  { field: 'tags', header: 'Tags' },
];

const COLUMNS: Record<string, Column[]> = {
  [RESOURCE_KEYS.node]: [
    { field: 'node', header: 'Node' },
    statusColumn,
    { field: 'maxcpu', header: 'CPUs' },
    { field: 'mem', header: 'Memory', bytes: true },
    { field: 'maxmem', header: 'Max memory', bytes: true },
    { field: 'maxdisk', header: 'Root disk', bytes: true },
  ],
  [RESOURCE_KEYS.storage]: [
    { field: 'storage', header: 'Storage' },
    { field: 'node', header: 'Node' },
    statusColumn,
    { field: 'plugintype', header: 'Type' },
    { field: 'content', header: 'Content' },
    { field: 'disk', header: 'Used', bytes: true },
    { field: 'maxdisk', header: 'Size', bytes: true },
  ],
  [RESOURCE_KEYS.virtualMachine]: guestColumns,
  [RESOURCE_KEYS.container]: guestColumns,
};

/** Kinds without their own columns (none today) still show their Proxmox ID. */
const FALLBACK_COLUMNS: Column[] = [{ field: 'id', header: 'ID' }];

export function columnsFor(resourceKey: string): Column[] {
  return COLUMNS[resourceKey] ?? FALLBACK_COLUMNS;
}

function present(value: unknown): boolean {
  return value !== undefined && value !== null && value !== '';
}

function stringify(value: unknown): string {
  if (!present(value)) return '';
  if (Array.isArray(value)) return value.map(stringify).join(', ');
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];

export function formatBytes(bytes: number): string {
  let value = bytes;
  let unit = 0;
  while (Math.abs(value) >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${unit === 0 ? value : value.toFixed(1)} ${BYTE_UNITS[unit]}`;
}

/** Renders a raw cell value as text. */
export function formatValue(value: unknown, col: Column): string {
  if (col.bytes && typeof value === 'number') return formatBytes(value);
  return stringify(value);
}

/** Value used for sorting: numbers stay numeric so memory/disk sort by magnitude. */
export function sortValue(value: unknown): string | number | undefined {
  if (!present(value)) return undefined;
  if (typeof value === 'number') return value;
  if (typeof value === 'boolean') return value ? 1 : 0;
  return stringify(value).toLowerCase();
}

/** Proxmox node, guest and storage `status` values. Anything else is neutral. */
const STATUS_BY_VALUE: Record<string, StatusDotStatus> = {
  online: 'success',
  running: 'success',
  available: 'success',
  offline: 'danger',
  stopped: 'neutral',
  paused: 'warning',
  unknown: 'warning',
};

export function columnStatus(value: unknown, col: Column): StatusDotStatus | undefined {
  if (!col.status) return undefined;
  return STATUS_BY_VALUE[stringify(value)] ?? 'neutral';
}

/** Human label for a Proxmox entry: guests by name and VMID, storage by pool and node. */
export function displayName(data: ProxmoxObject): string {
  const { name, vmid, storage, node } = data;
  if (present(vmid)) return present(name) ? `${stringify(name)} (${stringify(vmid)})` : stringify(vmid);
  if (present(storage)) return present(node) ? `${stringify(storage)} on ${stringify(node)}` : stringify(storage);
  if (present(node)) return stringify(node);
  return data.id;
}
