import type { StatusDotStatus } from '@omniviewdev/base-ui';

import { RESOURCE_KEYS } from '../constants';
import { monitorCount } from './cephStatus';
import { parseStorageId, refsOf } from './refs';

/** A native Proxmox API object, passed through unmodified by the backend. */
export type NativeObject = Record<string, unknown>;

/** A Proxmox `/cluster/resources` entry; `id` is Proxmox's cluster-unique ID. */
export type ProxmoxObject = NativeObject & { id: string };

/**
 * One table column. Columns live here rather than in the backend's
 * ResourceDefinition: the resource transport has no RPC that delivers
 * definitions to the UI.
 */
export type Column = {
  /** Field of the native object; dots reach into nested objects (`health.status`). */
  field: string;
  header: string;
  /** Byte count, rendered as KiB/MiB/… and right-aligned. */
  bytes?: boolean;
  /** Fraction 0–1, rendered as a percentage and right-aligned. */
  ratio?: boolean;
  /** Rendered as a status dot coloured by STATUS_BY_VALUE. */
  status?: boolean;
  /** Derives the cell value instead of reading `field`, which stays the column ID. */
  value?: (row: NativeObject) => unknown;
};

/** Storage a guest uses, by name; storage on another node than the guest's is shown with that node. */
function guestStorage(row: NativeObject): string[] | undefined {
  return refsOf(row).storage?.map((id) => {
    const { node, storage } = parseStorageId(id);
    return node === row.node ? storage : `${storage} (${node})`;
  });
}

/** How one resource kind is shown. */
export type Kind = {
  columns: Column[];
  /** Field the backend identifies objects by; Ceph objects have no Proxmox `id`. */
  idField: string;
};

const statusColumn: Column = { field: 'status', header: 'Status', status: true };
const versionColumn: Column = { field: 'ceph_version_short', header: 'Version' };

const guestColumns: Column[] = [
  { field: 'vmid', header: 'VMID' },
  { field: 'name', header: 'Name' },
  { field: 'node', header: 'Node' },
  statusColumn,
  { field: 'maxcpu', header: 'vCPUs' },
  { field: 'mem', header: 'Memory', bytes: true },
  { field: 'maxmem', header: 'Max memory', bytes: true },
  { field: 'maxdisk', header: 'Disk', bytes: true },
  { field: 'omniview.storage', header: 'Storage', value: guestStorage },
  { field: 'tags', header: 'Tags' },
];

const KINDS: Record<string, Kind> = {
  [RESOURCE_KEYS.node]: {
    idField: 'id',
    columns: [
      { field: 'node', header: 'Node' },
      statusColumn,
      { field: 'maxcpu', header: 'CPUs' },
      { field: 'mem', header: 'Memory', bytes: true },
      { field: 'maxmem', header: 'Max memory', bytes: true },
      { field: 'maxdisk', header: 'Root disk', bytes: true },
    ],
  },
  [RESOURCE_KEYS.storage]: {
    idField: 'id',
    columns: [
      { field: 'storage', header: 'Storage' },
      { field: 'node', header: 'Node' },
      statusColumn,
      { field: 'plugintype', header: 'Type' },
      { field: 'content', header: 'Content' },
      { field: 'disk', header: 'Used', bytes: true },
      { field: 'maxdisk', header: 'Size', bytes: true },
    ],
  },
  [RESOURCE_KEYS.virtualMachine]: { idField: 'id', columns: guestColumns },
  [RESOURCE_KEYS.container]: { idField: 'id', columns: guestColumns },
  // `ceph status` output.
  [RESOURCE_KEYS.cephCluster]: {
    idField: 'fsid',
    columns: [
      { field: 'fsid', header: 'FSID' },
      { field: 'health.status', header: 'Health', status: true },
      { field: 'monmap.mons', header: 'Monitors', value: monitorCount },
      { field: 'osdmap.num_osds', header: 'OSDs' },
      { field: 'osdmap.num_up_osds', header: 'OSDs up' },
      { field: 'osdmap.num_in_osds', header: 'OSDs in' },
      { field: 'pgmap.num_pools', header: 'Pools' },
      { field: 'pgmap.num_pgs', header: 'PGs' },
      { field: 'pgmap.bytes_used', header: 'Used', bytes: true },
      { field: 'pgmap.bytes_total', header: 'Size', bytes: true },
    ],
  },
  [RESOURCE_KEYS.cephPool]: {
    idField: 'pool_name',
    columns: [
      { field: 'pool_name', header: 'Pool' },
      { field: 'type', header: 'Type' },
      { field: 'size', header: 'Size' },
      { field: 'min_size', header: 'Min size' },
      { field: 'pg_num', header: 'PGs' },
      { field: 'pg_autoscale_mode', header: 'Autoscale' },
      { field: 'crush_rule_name', header: 'CRUSH rule' },
      { field: 'bytes_used', header: 'Used', bytes: true },
      { field: 'percent_used', header: 'Used %', ratio: true },
    ],
  },
  // Leaves of the `/nodes/{node}/ceph/osd` CRUSH tree.
  [RESOURCE_KEYS.cephOSD]: {
    idField: 'name',
    columns: [
      { field: 'name', header: 'OSD' },
      { field: 'host', header: 'Host' },
      statusColumn,
      { field: 'in', header: 'In' },
      { field: 'device_class', header: 'Class' },
      { field: 'crush_weight', header: 'Weight' },
      { field: 'bytes_used', header: 'Used', bytes: true },
      { field: 'total_space', header: 'Size', bytes: true },
      { field: 'apply_latency_ms', header: 'Latency (ms)' },
      versionColumn,
    ],
  },
  [RESOURCE_KEYS.cephMonitor]: {
    idField: 'name',
    columns: [
      { field: 'name', header: 'Monitor' },
      { field: 'host', header: 'Host' },
      { field: 'state', header: 'State', status: true },
      { field: 'quorum', header: 'Quorum' },
      { field: 'addr', header: 'Address' },
      versionColumn,
    ],
  },
  [RESOURCE_KEYS.cephManager]: {
    idField: 'name',
    columns: [
      { field: 'name', header: 'Manager' },
      { field: 'host', header: 'Host' },
      { field: 'state', header: 'State', status: true },
      { field: 'addr', header: 'Address' },
      versionColumn,
    ],
  },
  [RESOURCE_KEYS.cephMetadataServer]: {
    idField: 'name',
    columns: [
      { field: 'name', header: 'MDS' },
      { field: 'host', header: 'Host' },
      { field: 'state', header: 'State', status: true },
      { field: 'fs_name', header: 'File system' },
      { field: 'rank', header: 'Rank' },
      { field: 'addr', header: 'Address' },
      versionColumn,
    ],
  },
  [RESOURCE_KEYS.cephFilesystem]: {
    idField: 'name',
    columns: [
      { field: 'name', header: 'File system' },
      { field: 'metadata_pool', header: 'Metadata pool' },
      { field: 'data_pools', header: 'Data pools' },
    ],
  },
};

/** Kinds without their own entry (none today) still show their Proxmox ID. */
const FALLBACK_KIND: Kind = { idField: 'id', columns: [{ field: 'id', header: 'ID' }] };

export function kindFor(resourceKey: string): Kind {
  return KINDS[resourceKey] ?? FALLBACK_KIND;
}

/** Value of a possibly dotted field. */
export function fieldValue(data: NativeObject, field: string): unknown {
  let value: unknown = data;
  for (const part of field.split('.')) {
    if (typeof value !== 'object' || value === null) return undefined;
    value = (value as NativeObject)[part];
  }
  return value;
}

/** A column's cell value: derived, or read from its field. */
export function cellValue(row: NativeObject, col: Column): unknown {
  return col.value ? col.value(row) : fieldValue(row, col.field);
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

/** The ID the backend knows an object by. */
export function objectId(data: NativeObject, kind: Kind): string {
  return stringify(fieldValue(data, kind.idField));
}

const BYTE_UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];

export function formatBytes(bytes: number): string {
  let value = bytes;
  let unit = 0;
  while (Math.abs(value) >= 1024 && unit < BYTE_UNITS.length - 1) {
    value /= 1024;
    unit += 1;
  }
  // Rates (bytes/s averaged by Proxmox) are fractional; whole bytes are enough.
  return `${unit === 0 ? Math.round(value) : value.toFixed(1)} ${BYTE_UNITS[unit]}`;
}

/** `12d 4h`, `3h 20m`, `45m`; under a minute is `<1m`. */
export function formatDuration(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return minutes > 0 ? `${minutes}m` : '<1m';
}

/** Numeric columns are right-aligned. */
export function isNumeric(col: Column): boolean {
  return Boolean(col.bytes || col.ratio);
}

/** Renders a raw cell value as text. */
export function formatValue(value: unknown, col: Column): string {
  if (col.bytes && typeof value === 'number') return formatBytes(value);
  if (col.ratio && typeof value === 'number') return `${(value * 100).toFixed(1)} %`;
  return stringify(value);
}

/** Value used for sorting: numbers stay numeric so memory/disk sort by magnitude. */
export function sortValue(value: unknown): string | number | undefined {
  if (!present(value)) return undefined;
  if (typeof value === 'number') return value;
  if (typeof value === 'boolean') return value ? 1 : 0;
  return stringify(value).toLowerCase();
}

/** Proxmox and Ceph status values. Anything else is neutral. */
const STATUS_BY_VALUE: Record<string, StatusDotStatus> = {
  online: 'success',
  running: 'success',
  available: 'success',
  offline: 'danger',
  stopped: 'neutral',
  paused: 'warning',
  unknown: 'warning',
  // Ceph health, OSDs, managers and metadata servers.
  HEALTH_OK: 'success',
  HEALTH_WARN: 'warning',
  HEALTH_ERR: 'danger',
  up: 'success',
  down: 'danger',
  active: 'success',
  standby: 'info',
  'up:active': 'success',
  'up:standby': 'info',
  'up:standby-replay': 'info',
};

/** Status dot colour of a status value. */
export function statusOf(value: unknown): StatusDotStatus {
  return STATUS_BY_VALUE[stringify(value)] ?? 'neutral';
}

export function columnStatus(value: unknown, col: Column): StatusDotStatus | undefined {
  return col.status ? statusOf(value) : undefined;
}

/** Human label: guests by name and VMID, storage by pool and node, Ceph objects by their ID. */
export function displayName(data: NativeObject, kind: Kind): string {
  if (kind.idField !== 'id') return objectId(data, kind);
  const { name, vmid, storage, node } = data;
  if (present(vmid)) return present(name) ? `${stringify(name)} (${stringify(vmid)})` : stringify(vmid);
  if (present(storage)) return present(node) ? `${stringify(storage)} on ${stringify(node)}` : stringify(storage);
  if (present(node)) return stringify(node);
  return objectId(data, kind);
}
