import type { ProxmoxObject } from './columns';

export type Count = { up: number; total: number };

export type ClusterSummary = {
  /** Nodes with `status == online` out of all nodes. */
  nodes: Count;
  /** Running QEMU guests out of all non-template QEMU guests. */
  virtualMachines: Count;
  /** Running LXC guests out of all non-template LXC guests. */
  containers: Count;
  /** Share of online nodes' CPUs in use, 0–1; undefined without online nodes. */
  cpu?: number;
  /** Share of online nodes' memory in use, 0–1; undefined without online nodes. */
  memory?: number;
};

function num(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

/** Templates cannot run, so counting them would make a healthy cluster look partly stopped. */
function countGuests(guests: ProxmoxObject[]): Count {
  const real = guests.filter((g) => num(g.template) !== 1);
  return { up: real.filter((g) => g.status === 'running').length, total: real.length };
}

/**
 * Cluster-wide health from `/cluster/resources` entries. Node `cpu` is the
 * busy fraction of that node's `maxcpu`, so cluster CPU is weighted by core count.
 */
export function summarizeCluster(
  nodes: ProxmoxObject[],
  virtualMachines: ProxmoxObject[],
  containers: ProxmoxObject[],
): ClusterSummary {
  const online = nodes.filter((n) => n.status === 'online');
  const cores = online.reduce((sum, n) => sum + num(n.maxcpu), 0);
  const busy = online.reduce((sum, n) => sum + num(n.cpu) * num(n.maxcpu), 0);
  const maxMemory = online.reduce((sum, n) => sum + num(n.maxmem), 0);
  const usedMemory = online.reduce((sum, n) => sum + num(n.mem), 0);

  return {
    nodes: { up: online.length, total: nodes.length },
    virtualMachines: countGuests(virtualMachines),
    containers: countGuests(containers),
    cpu: cores > 0 ? busy / cores : undefined,
    memory: maxMemory > 0 ? usedMemory / maxMemory : undefined,
  };
}
