import type { StatusDotStatus, TableCellAlign } from '@omniviewdev/base-ui';
import type { resource } from '@omniviewdev/runtime/models';

/** A Proxmox `/cluster/resources` entry, passed through unmodified by the backend. */
export type ProxmoxObject = Record<string, unknown> & { id: string };

export type ColumnDefinition = resource.ColumnDefinition;

/** Columns used when the backend definition declares none. */
const FALLBACK_COLUMNS = [{ id: 'id', header: 'ID', accessor: 'id' }] as ColumnDefinition[];

export function visibleColumns(defs: ColumnDefinition[] | undefined): ColumnDefinition[] {
  const visible = (defs ?? []).filter((col) => !col.hidden);
  return visible.length > 0 ? visible : FALLBACK_COLUMNS;
}

function readPath(data: unknown, path: string): unknown {
  const segments = path
    .replace(/^\$\.?/, '')
    .replace(/\[(\d+)\]/g, '.$1')
    .split('.')
    .filter(Boolean);
  let current = data;
  for (const segment of segments) {
    if (current === null || typeof current !== 'object') return undefined;
    current = (current as Record<string, unknown>)[segment];
  }
  return current;
}

function present(value: unknown): boolean {
  return value !== undefined && value !== null && value !== '';
}

/** Resolves a column's comma-separated accessors honouring its accessor priority. */
export function columnValue(data: ProxmoxObject, col: ColumnDefinition): unknown {
  const values = col.accessor
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((path) => readPath(data, path))
    .filter(present);
  if (values.length === 0) return undefined;
  switch ((col.accessorPriority ?? '').toUpperCase()) {
    case 'LAST':
      return values[values.length - 1];
    case 'ALL':
      return values.length === 1 ? values[0] : values;
    default:
      return values[0];
  }
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

/** Formats a duration given in seconds, e.g. Proxmox `uptime`. */
export function formatDuration(totalSeconds: number): string {
  if (totalSeconds <= 0) return '0s';
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = Math.floor(totalSeconds % 60);
  const parts = [
    days && `${days}d`,
    hours && `${hours}h`,
    minutes && `${minutes}m`,
    !days && !hours && seconds && `${seconds}s`,
  ].filter(Boolean);
  return parts.slice(0, 2).join(' ') || '0s';
}

/** Numbers below 1e12 are Unix seconds (Proxmox convention); larger ones milliseconds. */
function toDate(value: unknown): Date | undefined {
  if (typeof value === 'number') return new Date(value < 1e12 ? value * 1000 : value);
  if (typeof value === 'string') {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? undefined : date;
  }
  return undefined;
}

function toNumber(value: unknown): number | undefined {
  if (typeof value === 'number') return value;
  if (typeof value === 'string' && value.trim() !== '' && !Number.isNaN(Number(value))) return Number(value);
  return undefined;
}

function aggregate(value: unknown, formatter: string): unknown {
  const items = Array.isArray(value) ? value : [value];
  if (formatter === 'COUNT') return items.length;
  const numbers = items.map(toNumber).filter((n): n is number => n !== undefined);
  if (numbers.length === 0) return undefined;
  switch (formatter) {
    case 'SUM':
      return numbers.reduce((a, b) => a + b, 0);
    case 'AVG':
      return numbers.reduce((a, b) => a + b, 0) / numbers.length;
    case 'MAX':
      return Math.max(...numbers);
    default:
      return Math.min(...numbers);
  }
}

function stringify(value: unknown): string {
  if (!present(value)) return '';
  if (Array.isArray(value)) return value.map(stringify).join(', ');
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function applyValueMap(text: string, valueMap: ColumnDefinition['valueMap']): string {
  let result = text;
  for (const [pattern, replacement] of Object.entries(valueMap ?? {})) {
    try {
      result = result.replace(new RegExp(pattern), replacement ?? '');
    } catch {
      // A malformed backend pattern leaves the value unchanged.
    }
  }
  return result;
}

/** Renders a raw cell value as text according to the column's backend formatter. */
export function formatValue(value: unknown, col: ColumnDefinition): string {
  if (!present(value)) return '';
  const formatter = (col.formatter ?? '').toUpperCase();
  let text: string;
  switch (formatter) {
    case 'BYTES': {
      const n = toNumber(value);
      text = n === undefined ? stringify(value) : formatBytes(n);
      break;
    }
    case 'PERCENT': {
      // Proxmox reports utilisation (cpu, …) as a 0..1 fraction.
      const n = toNumber(value);
      text = n === undefined ? stringify(value) : `${(n * 100).toFixed(1)}%`;
      break;
    }
    case 'DURATION': {
      const n = toNumber(value);
      text = n === undefined ? stringify(value) : formatDuration(n);
      break;
    }
    case 'AGE': {
      const date = toDate(value);
      text = date ? formatDuration((Date.now() - date.getTime()) / 1000) : stringify(value);
      break;
    }
    case 'TIME': {
      const date = toDate(value);
      text = date ? date.toLocaleString() : stringify(value);
      break;
    }
    case 'SUM':
    case 'COUNT':
    case 'AVG':
    case 'MAX':
    case 'MIN':
      text = stringify(aggregate(value, formatter));
      break;
    default:
      text = stringify(value);
  }
  return applyValueMap(text, col.valueMap);
}

/** Value used for sorting: numbers stay numeric so memory/disk sort by magnitude. */
export function sortValue(value: unknown): string | number | undefined {
  if (!present(value)) return undefined;
  if (typeof value === 'number') return value;
  if (typeof value === 'boolean') return value ? 1 : 0;
  return stringify(value).toLowerCase();
}

const STATUS_BY_COLOR: Record<string, StatusDotStatus> = {
  primary: 'info',
  info: 'info',
  success: 'success',
  warning: 'warning',
  danger: 'danger',
  error: 'danger',
  neutral: 'neutral',
};

/** Resolves the column's colour variant (`colorMap` with `*` wildcard, then `color`). */
export function columnStatus(value: unknown, col: ColumnDefinition): StatusDotStatus | undefined {
  const map = col.colorMap ?? {};
  const hasMap = Object.keys(map).length > 0;
  const color = (hasMap ? map[stringify(value)] ?? map['*'] : undefined) ?? col.color;
  return color ? STATUS_BY_COLOR[color.toLowerCase()] ?? 'neutral' : undefined;
}

export function columnAlign(col: ColumnDefinition): TableCellAlign {
  switch ((col.align ?? '').toUpperCase()) {
    case 'RIGHT':
      return 'right';
    case 'CENTER':
      return 'center';
    default:
      return 'left';
  }
}

/** Human label for a Proxmox entry: guests by name and VMID, storage by pool and node. */
export function displayName(data: ProxmoxObject): string {
  const { name, vmid, storage, node } = data;
  if (present(vmid)) return present(name) ? `${stringify(name)} (${stringify(vmid)})` : stringify(vmid);
  if (present(storage)) return present(node) ? `${stringify(storage)} on ${stringify(node)}` : stringify(storage);
  if (present(node)) return stringify(node);
  return data.id;
}
