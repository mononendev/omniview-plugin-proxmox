import type { NativeObject } from './columns';

/**
 * Proxmox's `/nodes/{node}/ceph/status` is `ceph status` with `health`
 * replaced by `ceph health detail`, `monmap` by the full monitor map and
 * `mgrmap` by `ceph mgr dump`. summarizeCephStatus reduces it to what an
 * operator reads `ceph -s` for. Every field is optional: older releases and
 * partial answers leave parts out.
 */

export type HealthCheck = {
  /** Ceph's check code, e.g. MON_DISK_LOW. */
  code: string;
  severity: string;
  summary: string;
  details: string[];
  muted: boolean;
};

export type Capacity = {
  used: number;
  total: number;
  available?: number;
  /** Bytes stored by clients, before replication. */
  stored?: number;
  objects?: number;
};

export type MonitorSummary = { total: number; inQuorum: string[]; outOfQuorum: string[]; quorumAgeSeconds?: number };
export type ManagerSummary = { active?: string; available?: boolean; standbys: string[]; standbyCount: number };
export type OsdSummary = { total: number; up: number; in: number };
export type MdsDaemon = { name: string; state: string };
export type MdsSummary = { ranks: MdsDaemon[]; standbyCount: number };
export type PgState = { state: string; count: number };

export type PgSummary = {
  total: number;
  pools?: number;
  byState: PgState[];
  /** Fractions 0–1 of degraded and misplaced objects, when there are any. */
  degraded?: number;
  misplaced?: number;
};

export type IoSummary = {
  readBytes: number;
  writeBytes: number;
  readOps: number;
  writeOps: number;
  recoveryBytes?: number;
  recoveryObjects?: number;
};

export type ProgressEvent = { message: string; fraction: number };

export type CephStatusSummary = {
  fsid: string;
  release?: string;
  health: { status: string; checks: HealthCheck[] };
  capacity?: Capacity;
  monitors?: MonitorSummary;
  managers?: ManagerSummary;
  osds?: OsdSummary;
  mds?: MdsSummary;
  /** Other daemons from the service map (rgw, rbd-mirror, …) by service, with their count. */
  services: { name: string; count: number }[];
  pgs?: PgSummary;
  io?: IoSummary;
  progress: ProgressEvent[];
};

function obj(value: unknown): NativeObject | undefined {
  return typeof value === 'object' && value !== null && !Array.isArray(value) ? (value as NativeObject) : undefined;
}

function arr(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function num(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function str(value: unknown): string | undefined {
  return typeof value === 'string' && value !== '' ? value : undefined;
}

function names(list: unknown): string[] {
  return arr(list).flatMap((item) => {
    const name = typeof item === 'string' ? item : str(obj(item)?.name);
    return name ? [name] : [];
  });
}

/** Monitor count: `num_mons` in plain `ceph status`, the map's `mons` in Proxmox's. */
export function monitorCount(status: NativeObject): number | undefined {
  const monmap = obj(status.monmap);
  return num(monmap?.num_mons) ?? (Array.isArray(monmap?.mons) ? monmap.mons.length : undefined);
}

function healthOf(status: NativeObject): CephStatusSummary['health'] {
  const health = obj(status.health);
  const checks = Object.entries(obj(health?.checks) ?? {}).map(([code, raw]): HealthCheck => {
    const check = obj(raw);
    return {
      code,
      severity: str(check?.severity) ?? 'HEALTH_WARN',
      summary: str(obj(check?.summary)?.message) ?? code,
      details: arr(check?.detail).flatMap((d) => {
        const message = str(obj(d)?.message);
        return message ? [message] : [];
      }),
      muted: check?.muted === true,
    };
  });
  // Errors before warnings, muted last.
  const rank = (c: HealthCheck) => (c.muted ? 2 : c.severity === 'HEALTH_ERR' ? 0 : 1);
  checks.sort((a, b) => rank(a) - rank(b) || a.code.localeCompare(b.code));
  return { status: str(health?.status) ?? 'unknown', checks };
}

function capacityOf(pgmap: NativeObject | undefined): Capacity | undefined {
  const used = num(pgmap?.bytes_used);
  const total = num(pgmap?.bytes_total);
  if (used === undefined || !total) return undefined;
  return {
    used,
    total,
    available: num(pgmap?.bytes_avail),
    stored: num(pgmap?.data_bytes),
    objects: num(pgmap?.num_objects),
  };
}

function monitorsOf(status: NativeObject): MonitorSummary | undefined {
  const all = names(obj(status.monmap)?.mons);
  const inQuorum = names(status.quorum_names);
  const total = monitorCount(status) ?? inQuorum.length;
  if (total === 0) return undefined;
  return {
    total,
    inQuorum,
    outOfQuorum: all.filter((name) => !inQuorum.includes(name)),
    quorumAgeSeconds: num(status.quorum_age),
  };
}

function managersOf(status: NativeObject): ManagerSummary | undefined {
  const mgrmap = obj(status.mgrmap);
  if (!mgrmap) return undefined;
  const standbys = names(mgrmap.standbys);
  return {
    active: str(mgrmap.active_name),
    available: typeof mgrmap.available === 'boolean' ? mgrmap.available : undefined,
    standbys,
    standbyCount: num(mgrmap.num_standbys) ?? standbys.length,
  };
}

function osdsOf(status: NativeObject): OsdSummary | undefined {
  const osdmap = obj(status.osdmap);
  const total = num(osdmap?.num_osds);
  if (total === undefined) return undefined;
  return { total, up: num(osdmap?.num_up_osds) ?? 0, in: num(osdmap?.num_in_osds) ?? 0 };
}

function mdsOf(status: NativeObject): MdsSummary | undefined {
  const fsmap = obj(status.fsmap);
  if (!fsmap) return undefined;
  const ranks = arr(fsmap.by_rank).flatMap((raw): MdsDaemon[] => {
    const rank = obj(raw);
    const name = str(rank?.name);
    return name ? [{ name, state: str(rank?.status) ?? 'unknown' }] : [];
  });
  const standbyCount = num(fsmap['up:standby']) ?? 0;
  if (ranks.length === 0 && standbyCount === 0) return undefined;
  return { ranks, standbyCount };
}

function servicesOf(status: NativeObject): CephStatusSummary['services'] {
  const services = obj(obj(status.servicemap)?.services) ?? {};
  return Object.entries(services)
    .map(([name, raw]) => ({
      name,
      // `daemons` holds one entry per daemon plus a `summary` string.
      count: Object.keys(obj(obj(raw)?.daemons) ?? {}).filter((key) => key !== 'summary').length,
    }))
    .filter((service) => service.count > 0)
    .sort((a, b) => a.name.localeCompare(b.name));
}

function pgsOf(pgmap: NativeObject | undefined): PgSummary | undefined {
  const total = num(pgmap?.num_pgs);
  if (total === undefined) return undefined;
  const byState = arr(pgmap?.pgs_by_state)
    .flatMap((raw): PgState[] => {
      const entry = obj(raw);
      const state = str(entry?.state_name);
      const count = num(entry?.count);
      return state && count !== undefined ? [{ state, count }] : [];
    })
    .sort((a, b) => b.count - a.count);
  return {
    total,
    pools: num(pgmap?.num_pools),
    byState,
    degraded: num(pgmap?.degraded_ratio) || undefined,
    misplaced: num(pgmap?.misplaced_ratio) || undefined,
  };
}

function ioOf(pgmap: NativeObject | undefined): IoSummary | undefined {
  if (!pgmap) return undefined;
  // Ceph omits idle rates, so a missing rate is zero.
  return {
    readBytes: num(pgmap.read_bytes_sec) ?? 0,
    writeBytes: num(pgmap.write_bytes_sec) ?? 0,
    readOps: num(pgmap.read_op_per_sec) ?? 0,
    writeOps: num(pgmap.write_op_per_sec) ?? 0,
    recoveryBytes: num(pgmap.recovering_bytes_per_sec),
    recoveryObjects: num(pgmap.recovering_objects_per_sec),
  };
}

function progressOf(status: NativeObject): ProgressEvent[] {
  return Object.values(obj(status.progress_events) ?? {}).flatMap((raw): ProgressEvent[] => {
    const event = obj(raw);
    const message = str(event?.message);
    return message ? [{ message, fraction: num(event?.progress) ?? 0 }] : [];
  });
}

export function summarizeCephStatus(status: NativeObject): CephStatusSummary {
  const pgmap = obj(status.pgmap);
  return {
    fsid: str(status.fsid) ?? '',
    release: str(obj(status.monmap)?.min_mon_release_name),
    health: healthOf(status),
    capacity: capacityOf(pgmap),
    monitors: monitorsOf(status),
    managers: managersOf(status),
    osds: osdsOf(status),
    mds: mdsOf(status),
    services: servicesOf(status),
    pgs: pgsOf(pgmap),
    io: ioOf(pgmap),
    progress: progressOf(status),
  };
}
