import { RESOURCE_KEYS } from '../constants';

export type ConfigOption = { value: string; label: string };

/** One editable key of a guest's `/config`. */
export type ConfigField = {
  key: string;
  label: string;
  kind: 'text' | 'textarea' | 'integer' | 'switch' | 'select';
  description?: string;
  /** Shown when the key is unset: PVE's default. */
  placeholder?: string;
  min?: number;
  options?: ConfigOption[];
};

/** Form values: text for inputs and selects, booleans for switches. */
export type ConfigValues = Record<string, string | boolean>;

/** Select value standing for "key unset" (base-ui items need a non-empty value). */
export const UNSET = '__unset__';

const CPU_TYPES: ConfigOption[] = [
  { value: UNSET, label: 'Default (kvm64)' },
  { value: 'host', label: 'host' },
  { value: 'x86-64-v2-AES', label: 'x86-64-v2-AES' },
  { value: 'x86-64-v2', label: 'x86-64-v2' },
  { value: 'x86-64-v3', label: 'x86-64-v3' },
  { value: 'x86-64-v4', label: 'x86-64-v4' },
  { value: 'kvm64', label: 'kvm64' },
  { value: 'qemu64', label: 'qemu64' },
  { value: 'max', label: 'max' },
];

const OS_TYPES: ConfigOption[] = [
  { value: UNSET, label: 'Default (other)' },
  { value: 'l26', label: 'Linux 6.x – 2.6 kernel' },
  { value: 'l24', label: 'Linux 2.4 kernel' },
  { value: 'win11', label: 'Windows 11/2022/2025' },
  { value: 'win10', label: 'Windows 10/2016/2019' },
  { value: 'win8', label: 'Windows 8/2012/2012r2' },
  { value: 'win7', label: 'Windows 7/2008r2' },
  { value: 'w2k8', label: 'Windows Vista/2008' },
  { value: 'wxp', label: 'Windows XP/2003' },
  { value: 'w2k', label: 'Windows 2000' },
  { value: 'solaris', label: 'Solaris kernel' },
  { value: 'other', label: 'Other' },
];

const description: ConfigField = { key: 'description', label: 'Notes', kind: 'textarea' };
const tags: ConfigField = {
  key: 'tags',
  label: 'Tags',
  kind: 'text',
  description: 'Separated by semicolons, commas or spaces.',
};
const onboot: ConfigField = { key: 'onboot', label: 'Start at boot', kind: 'switch' };
const protection: ConfigField = {
  key: 'protection',
  label: 'Protection',
  kind: 'switch',
  description: 'Blocks removing the guest and its disks.',
};
const cores: ConfigField = { key: 'cores', label: 'Cores', kind: 'integer', min: 1, placeholder: '1' };

const VM_FIELDS: ConfigField[] = [
  { key: 'name', label: 'Name', kind: 'text' },
  description,
  tags,
  onboot,
  protection,
  cores,
  { key: 'sockets', label: 'Sockets', kind: 'integer', min: 1, placeholder: '1' },
  { key: 'cpu', label: 'CPU type', kind: 'select', options: CPU_TYPES },
  { key: 'memory', label: 'Memory (MiB)', kind: 'integer', min: 16, placeholder: '512' },
  {
    key: 'balloon',
    label: 'Minimum memory (MiB)',
    kind: 'integer',
    min: 0,
    description: 'Ballooning target; 0 turns the balloon device off.',
    placeholder: 'Same as memory',
  },
  { key: 'ostype', label: 'Guest OS', kind: 'select', options: OS_TYPES },
];

const CT_FIELDS: ConfigField[] = [
  { key: 'hostname', label: 'Hostname', kind: 'text' },
  description,
  tags,
  onboot,
  protection,
  { ...cores, placeholder: 'All host cores' },
  { key: 'memory', label: 'Memory (MiB)', kind: 'integer', min: 16, placeholder: '512' },
  { key: 'swap', label: 'Swap (MiB)', kind: 'integer', min: 0, placeholder: '512' },
];

export function configFields(resourceKey: string): ConfigField[] {
  return resourceKey === RESOURCE_KEYS.container ? CT_FIELDS : VM_FIELDS;
}

/** A select's options, plus the current value when it is not one of them (e.g. `host,flags=+aes`). */
export function selectOptions(field: ConfigField, current: string): ConfigOption[] {
  const options = field.options ?? [];
  if (current === UNSET || options.some((o) => o.value === current)) return options;
  return [...options, { value: current, label: current }];
}

/** Form values of a `/config` response. */
export function initialValues(fields: ConfigField[], config: Record<string, unknown>): ConfigValues {
  const values: ConfigValues = {};
  for (const field of fields) {
    const raw = config[field.key];
    if (field.kind === 'switch') values[field.key] = raw === 1 || raw === true || raw === '1';
    else if (raw === undefined || raw === null || raw === '') values[field.key] = field.kind === 'select' ? UNSET : '';
    else values[field.key] = String(raw);
  }
  return values;
}

/** Client-side checks; keyed by config key. */
export function validate(fields: ConfigField[], values: ConfigValues): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const field of fields) {
    const value = values[field.key];
    if (field.kind !== 'integer' || typeof value !== 'string' || value.trim() === '') continue;
    if (!/^\d+$/.test(value.trim())) errors[field.key] = 'Enter a whole number.';
    else if (field.min !== undefined && Number(value) < field.min) errors[field.key] = `At least ${field.min}.`;
  }
  return errors;
}

/**
 * The keys that differ from `initial`, as the backend's Update input
 * expects them: numbers as numbers, switches as 0/1, and `''` to delete a key.
 */
export function changedValues(
  fields: ConfigField[],
  initial: ConfigValues,
  values: ConfigValues,
): Record<string, string | number> {
  const changes: Record<string, string | number> = {};
  for (const field of fields) {
    const value = values[field.key];
    if (value === initial[field.key]) continue;
    if (typeof value === 'boolean') changes[field.key] = value ? 1 : 0;
    else if (field.kind === 'select') changes[field.key] = value === UNSET ? '' : value;
    else if (field.kind === 'integer') changes[field.key] = value.trim() === '' ? '' : Number(value);
    else changes[field.key] = value;
  }
  return changes;
}

/**
 * Splits a backend INVALID message into per-key errors: PVE reports
 * parameter errors as `key: reason` pairs. Pairs for keys outside `fields`,
 * and any text that is not a pair, stay in `rest`.
 */
export function fieldErrors(fields: ConfigField[], message: string): { byKey: Record<string, string>; rest: string } {
  const keys = new Set(fields.map((f) => f.key));
  const byKey: Record<string, string> = {};
  const rest = message.replace(/\b([a-z][a-z0-9_-]*): ([^;\n]+)/gi, (match, key: string, reason: string) => {
    if (!keys.has(key)) return match;
    byKey[key] = reason.trim();
    return '';
  });
  return { byKey, rest: rest.replace(/[\s;,:]+$/, '').trim() };
}
