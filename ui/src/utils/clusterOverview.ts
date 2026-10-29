import { RESOURCE_KEYS } from '../constants';
import { type Capacity, type ClusterSummary, isTemplate, num, summarizeCluster } from './clusterSummary';
import type { ProxmoxObject } from './columns';

/** Guests of one type by state; `other` is paused, suspended, unknown and the like. Templates are not guests here. */
export type GuestBreakdown = { running: number; stopped: number; other: number; templates: number };

/** One storage as the cluster sees it: shared storage once, local storage once per node. */
export type StorageUsage = {
  /** Entry this row stands for; for shared storage, an available node's entry. */
  entry: ProxmoxObject;
  name: string;
  /** Node of local storage; undefined for shared storage. */
  node?: string;
  /** `name on node` for local storage, `name` for shared. */
  label: string;
  type: string;
  available: boolean;
  used: number;
  total: number;
  /** used / total, 0–1; undefined when the size is unknown. */
  fraction?: number;
};

export type NodeLoad = {
  entry: ProxmoxObject;
  name: string;
  online: boolean;
  cpu?: number;
  cores: number;
  memory: Capacity;
  uptime: number;
  /** Non-template guests placed on this node. */
  guests: { running: number; total: number };
};

export type GuestLoad = {
  resourceKey: string;
  entry: ProxmoxObject;
  name: string;
  /** Cores busy: the guest's `cpu` fraction times its vCPUs. */
  cores: number;
  /** Share of the guest's own vCPUs in use, 0–1. */
  cpu: number;
  vcpus: number;
  memory: Capacity;
};

export type Issue = { severity: 'warning' | 'danger'; message: string };

export type ClusterOverview = {
  summary: ClusterSummary;
  virtualMachines: GuestBreakdown;
  containers: GuestBreakdown;
  /** vCPUs and memory configured on running guests. */
  allocated: { vcpus: number; memory: number };
  /** Capacity of available storage; shared storage counted once. */
  storage: Capacity;
  nodes: NodeLoad[];
  storageEntries: StorageUsage[];
  /** Running guests by busy cores, busiest first. */
  topCpu: GuestLoad[];
  /** Running guests by memory in use, largest first. */
  topMemory: GuestLoad[];
  issues: Issue[];
};

/** Storage this full warns; STORAGE_CRITICAL is an error. */
export const STORAGE_WARNING = 0.85;
export const STORAGE_CRITICAL = 0.95;
/** Running guests listed per top list. */
export const TOP_GUESTS = 5;

const naturally = (a: string, b: string) => a.localeCompare(b, undefined, { numeric: true });

function str(value: unknown): string {
  return typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value);
}

function breakdown(guests: ProxmoxObject[]): GuestBreakdown {
  const out: GuestBreakdown = { running: 0, stopped: 0, other: 0, templates: 0 };
  for (const g of guests) {
    if (isTemplate(g)) out.templates += 1;
    else if (g.status === 'running') out.running += 1;
    else if (g.status === 'stopped') out.stopped += 1;
    else out.other += 1;
  }
  return out;
}

function guestName(g: ProxmoxObject): string {
  const name = str(g.name);
  const vmid = str(g.vmid);
  return name ? `${name} (${vmid})` : vmid || g.id;
}

/**
 * One row per storage: entries with `shared == 1` are the same storage seen
 * from every node, so they collapse to one, preferring an available entry.
 */
function collapseStorage(entries: ProxmoxObject[]): StorageUsage[] {
  const byKey = new Map<string, StorageUsage>();
  for (const e of entries) {
    const shared = num(e.shared) === 1;
    const name = str(e.storage) || e.id;
    const key = shared ? `shared/${name}` : e.id;
    const available = e.status === 'available';
    const prev = byKey.get(key);
    if (prev && (prev.available || !available)) continue;
    const used = num(e.disk);
    const total = num(e.maxdisk);
    const node = shared ? undefined : str(e.node) || undefined;
    byKey.set(key, {
      entry: e,
      name,
      node,
      label: node ? `${name} on ${node}` : name,
      type: str(e.plugintype),
      available,
      used,
      total,
      fraction: total > 0 ? used / total : undefined,
    });
  }
  return [...byKey.values()].sort(
    (a, b) => (b.fraction ?? -1) - (a.fraction ?? -1) || naturally(a.name, b.name) || naturally(a.node ?? '', b.node ?? ''),
  );
}

function names(list: string[]): string {
  return list.length <= 3 ? list.join(', ') : `${list.slice(0, 3).join(', ')} and ${list.length - 3} more`;
}

function plural(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`;
}

/** Everything the overview page shows, from the four watched `/cluster/resources` kinds. */
export function analyzeCluster(
  nodes: ProxmoxObject[],
  storage: ProxmoxObject[],
  virtualMachines: ProxmoxObject[],
  containers: ProxmoxObject[],
): ClusterOverview {
  const summary = summarizeCluster(nodes, virtualMachines, containers);
  const online = new Set(nodes.filter((n) => n.status === 'online').map((n) => str(n.node)));

  const guests: GuestLoad[] = [];
  const perNode = new Map<string, { running: number; total: number }>();
  const allocated = { vcpus: 0, memory: 0 };
  const stuck: string[] = [];
  const add = (resourceKey: string, list: ProxmoxObject[]) => {
    for (const g of list) {
      if (isTemplate(g)) continue;
      const node = str(g.node);
      const counts = perNode.get(node) ?? { running: 0, total: 0 };
      counts.total += 1;
      perNode.set(node, counts);
      if (g.status === 'running') {
        counts.running += 1;
        allocated.vcpus += num(g.maxcpu);
        allocated.memory += num(g.maxmem);
        guests.push({
          resourceKey,
          entry: g,
          name: guestName(g),
          cores: num(g.cpu) * num(g.maxcpu),
          cpu: num(g.cpu),
          vcpus: num(g.maxcpu),
          memory: { used: num(g.mem), total: num(g.maxmem) },
        });
      } else if (g.status !== 'stopped' && online.has(node)) {
        // A guest on an offline node is `unknown`; the node's own issue covers it.
        stuck.push(`${guestName(g)} is ${str(g.status) || 'in an unknown state'}`);
      }
    }
  };
  add(RESOURCE_KEYS.virtualMachine, virtualMachines);
  add(RESOURCE_KEYS.container, containers);

  const nodeLoads: NodeLoad[] = nodes
    .map((n) => {
      const isOnline = n.status === 'online';
      return {
        entry: n,
        name: str(n.node) || n.id,
        online: isOnline,
        cpu: isOnline && num(n.maxcpu) > 0 ? num(n.cpu) : undefined,
        cores: num(n.maxcpu),
        memory: { used: isOnline ? num(n.mem) : 0, total: num(n.maxmem) },
        uptime: isOnline ? num(n.uptime) : 0,
        guests: perNode.get(str(n.node)) ?? { running: 0, total: 0 },
      };
    })
    .sort((a, b) => naturally(a.name, b.name));

  const storageEntries = collapseStorage(storage);
  const capacity = storageEntries
    .filter((s) => s.available)
    .reduce((sum, s) => ({ used: sum.used + s.used, total: sum.total + s.total }), { used: 0, total: 0 });

  const issues: Issue[] = [];
  const offline = nodeLoads.filter((n) => !n.online).map((n) => n.name);
  if (offline.length > 0) {
    issues.push({ severity: 'danger', message: `${plural(offline.length, 'node')} offline: ${names(offline)}` });
  }
  for (const s of storageEntries) {
    if (s.fraction === undefined || s.fraction < STORAGE_WARNING) continue;
    issues.push({
      severity: s.fraction >= STORAGE_CRITICAL ? 'danger' : 'warning',
      message: `${s.label} is ${Math.round(s.fraction * 100)} % full`,
    });
  }
  const unavailable = storageEntries.filter((s) => !s.available && (s.node === undefined || online.has(s.node)));
  if (unavailable.length > 0) {
    issues.push({
      severity: 'warning',
      message: `${plural(unavailable.length, 'storage', 'storages')} unavailable: ${names(unavailable.map((s) => s.label))}`,
    });
  }
  if (stuck.length > 0) {
    issues.push({ severity: 'warning', message: `${plural(stuck.length, 'guest')} neither running nor stopped: ${names(stuck)}` });
  }

  return {
    summary,
    virtualMachines: breakdown(virtualMachines),
    containers: breakdown(containers),
    allocated,
    storage: capacity,
    nodes: nodeLoads,
    storageEntries,
    topCpu: [...guests].sort((a, b) => b.cores - a.cores || naturally(a.name, b.name)).slice(0, TOP_GUESTS),
    topMemory: [...guests].sort((a, b) => b.memory.used - a.memory.used || naturally(a.name, b.name)).slice(0, TOP_GUESTS),
    issues,
  };
}
