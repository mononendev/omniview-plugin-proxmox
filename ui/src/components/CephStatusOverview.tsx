import type { ReactNode } from 'react';
import { DescriptionList, Meter, StatusDot, type StatusDotStatus, Text } from '@omniviewdev/base-ui';

import { CEPH_USAGE_ZONES } from '../constants';
import {
  type CephStatusSummary,
  type MonitorSummary,
  summarizeCephStatus,
} from '../utils/cephStatus';
import { type NativeObject, formatBytes, formatDuration, statusOf } from '../utils/columns';
import styles from './CephStatusOverview.module.css';

/** Health detail lines shown per check; OSD_DOWN and the like can list hundreds. */
const MAX_DETAILS = 8;

const numberFormat = new Intl.NumberFormat();

function percent(fraction: number): string {
  return `${(fraction * 100).toFixed(1)} %`;
}

/** active+clean is healthy; PGs not serving I/O or with lost/damaged data are errors; anything else is in progress. */
function pgStateStatus(state: string): StatusDotStatus {
  if (state === 'active+clean') return 'success';
  if (!state.includes('active') || /down|incomplete|stale|inconsistent/.test(state)) return 'danger';
  return 'warning';
}

function monitorStatus(mons: MonitorSummary): StatusDotStatus {
  if (mons.inQuorum.length === 0) return 'danger';
  return mons.inQuorum.length < mons.total ? 'warning' : 'success';
}

function Section({ title, aside, children }: { title: string; aside?: ReactNode; children: ReactNode }) {
  return (
    <section className={styles.Section}>
      <div className={styles.SectionHeader}>
        <Text weight="semibold" size="sm">
          {title}
        </Text>
        {aside}
      </div>
      {children}
    </section>
  );
}

function Status({ status, children }: { status: StatusDotStatus; children: ReactNode }) {
  return (
    <span className={styles.Status}>
      <StatusDot status={status} size="sm" />
      <span>{children}</span>
    </span>
  );
}

function HealthSection({ health }: { health: CephStatusSummary['health'] }) {
  return (
    <Section title="Health" aside={<StatusDot status={statusOf(health.status)} label={health.status} size="sm" />}>
      {health.checks.length === 0 ? (
        <Text size="sm" tone="muted">
          No health checks raised.
        </Text>
      ) : (
        <ul className={styles.Checks}>
          {health.checks.map((check) => (
            <li key={check.code} className={styles.Check} data-muted={check.muted || undefined}>
              <StatusDot status={check.muted ? 'neutral' : statusOf(check.severity)} size="sm" />
              <div className={styles.CheckBody}>
                <Text size="sm">{check.summary}</Text>
                <Text size="xs" tone="muted" className={styles.Code}>
                  {check.code}
                  {check.muted ? ' · muted' : ''}
                </Text>
                {check.details.length > 0 && (
                  <ul className={styles.Details}>
                    {check.details.slice(0, MAX_DETAILS).map((detail) => (
                      <li key={detail}>{detail}</li>
                    ))}
                    {check.details.length > MAX_DETAILS && <li>and {check.details.length - MAX_DETAILS} more</li>}
                  </ul>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </Section>
  );
}

function CapacitySection({ capacity }: { capacity: NonNullable<CephStatusSummary['capacity']> }) {
  const fraction = capacity.used / capacity.total;
  return (
    <Section title="Capacity">
      <div className={styles.Usage}>
        <Meter size="sm" value={fraction * 100} {...CEPH_USAGE_ZONES} aria-label={`Raw capacity used ${percent(fraction)}`} />
        <Text size="sm">
          {formatBytes(capacity.used)} of {formatBytes(capacity.total)} used ({percent(fraction)})
        </Text>
      </div>
      <DescriptionList layout="horizontal" size="sm">
        {capacity.available !== undefined && (
          <DescriptionList.Item label="Available">{formatBytes(capacity.available)}</DescriptionList.Item>
        )}
        {capacity.stored !== undefined && (
          <DescriptionList.Item label="Stored data">{formatBytes(capacity.stored)} before replication</DescriptionList.Item>
        )}
        {capacity.objects !== undefined && (
          <DescriptionList.Item label="Objects">{numberFormat.format(capacity.objects)}</DescriptionList.Item>
        )}
      </DescriptionList>
    </Section>
  );
}

function ServicesSection({ summary }: { summary: CephStatusSummary }) {
  const { monitors, managers, osds, mds, services } = summary;
  return (
    <Section title="Services">
      <DescriptionList layout="horizontal" size="sm">
        {monitors && (
          <DescriptionList.Item label="Monitors">
            <Status status={monitorStatus(monitors)}>
              {monitors.inQuorum.length} of {monitors.total} in quorum
              {monitors.inQuorum.length > 0 && ` (${monitors.inQuorum.join(', ')})`}
              {monitors.outOfQuorum.length > 0 && `, out: ${monitors.outOfQuorum.join(', ')}`}
              {monitors.quorumAgeSeconds !== undefined && ` · formed ${formatDuration(monitors.quorumAgeSeconds)} ago`}
            </Status>
          </DescriptionList.Item>
        )}
        {managers && (
          <DescriptionList.Item label="Managers">
            <Status status={managers.active && managers.available !== false ? 'success' : 'danger'}>
              {managers.active ? `${managers.active} active` : 'no active manager'}
              {managers.standbyCount > 0 &&
                `, ${managers.standbyCount} standby${managers.standbys.length > 0 ? ` (${managers.standbys.join(', ')})` : ''}`}
            </Status>
          </DescriptionList.Item>
        )}
        {osds && (
          <DescriptionList.Item label="OSDs">
            <Status status={osds.up < osds.total || osds.in < osds.total ? 'warning' : 'success'}>
              {osds.up} up, {osds.in} in, of {osds.total}
            </Status>
          </DescriptionList.Item>
        )}
        {mds && (
          <DescriptionList.Item label="Metadata servers">
            <Status status={mds.ranks.every((r) => r.state === 'up:active') ? 'success' : 'warning'}>
              {mds.ranks.map((r) => (r.state === 'up:active' ? r.name : `${r.name} (${r.state})`)).join(', ') || 'none'}
              {mds.ranks.length > 0 && ' active'}
              {mds.standbyCount > 0 && `, ${mds.standbyCount} standby`}
            </Status>
          </DescriptionList.Item>
        )}
        {services.map((service) => (
          <DescriptionList.Item key={service.name} label={service.name}>
            {service.count} {service.count === 1 ? 'daemon' : 'daemons'}
          </DescriptionList.Item>
        ))}
      </DescriptionList>
    </Section>
  );
}

function PgSection({ pgs }: { pgs: NonNullable<CephStatusSummary['pgs']> }) {
  return (
    <Section
      title="Placement groups"
      aside={
        <Text size="sm" tone="muted">
          {numberFormat.format(pgs.total)} PGs{pgs.pools !== undefined && ` in ${pgs.pools} pools`}
        </Text>
      }
    >
      <ul className={styles.PgStates}>
        {pgs.byState.map(({ state, count }) => (
          <li key={state}>
            <Status status={pgStateStatus(state)}>
              <span className={styles.PgState}>{state}</span>
            </Status>
            <Text size="sm" className={styles.PgCount}>
              {numberFormat.format(count)}
            </Text>
          </li>
        ))}
      </ul>
      {(pgs.degraded !== undefined || pgs.misplaced !== undefined) && (
        <DescriptionList layout="horizontal" size="sm">
          {pgs.degraded !== undefined && (
            <DescriptionList.Item label="Degraded objects">{percent(pgs.degraded)}</DescriptionList.Item>
          )}
          {pgs.misplaced !== undefined && (
            <DescriptionList.Item label="Misplaced objects">{percent(pgs.misplaced)}</DescriptionList.Item>
          )}
        </DescriptionList>
      )}
    </Section>
  );
}

function IoSection({ io }: { io: NonNullable<CephStatusSummary['io']> }) {
  return (
    <Section title="Client I/O">
      <DescriptionList layout="horizontal" size="sm">
        <DescriptionList.Item label="Read">
          {formatBytes(io.readBytes)}/s · {numberFormat.format(io.readOps)} op/s
        </DescriptionList.Item>
        <DescriptionList.Item label="Write">
          {formatBytes(io.writeBytes)}/s · {numberFormat.format(io.writeOps)} op/s
        </DescriptionList.Item>
        {(io.recoveryBytes !== undefined || io.recoveryObjects !== undefined) && (
          <DescriptionList.Item label="Recovery">
            {formatBytes(io.recoveryBytes ?? 0)}/s · {numberFormat.format(io.recoveryObjects ?? 0)} objects/s
          </DescriptionList.Item>
        )}
      </DescriptionList>
    </Section>
  );
}

/** Detail view of `ceph status`: what `ceph -s` shows, laid out by concern. The JSON view keeps the raw maps. */
export default function CephStatusOverview({ data }: { data?: NativeObject }) {
  if (!data) return null;
  const summary = summarizeCephStatus(data);

  return (
    <div className={styles.Root}>
      <HealthSection health={summary.health} />
      {summary.capacity && <CapacitySection capacity={summary.capacity} />}
      <ServicesSection summary={summary} />
      {summary.pgs && <PgSection pgs={summary.pgs} />}
      {summary.io && <IoSection io={summary.io} />}
      {summary.progress.length > 0 && (
        <Section title="In progress">
          {summary.progress.map((event) => (
            <div key={event.message} className={styles.Usage}>
              <Meter size="sm" value={event.fraction * 100} color="info" aria-label={event.message} />
              <Text size="sm">
                {event.message} ({percent(event.fraction)})
              </Text>
            </div>
          ))}
        </Section>
      )}
      <Section title="Cluster">
        <DescriptionList layout="horizontal" size="sm">
          <DescriptionList.Item label="FSID" copyable>
            {summary.fsid}
          </DescriptionList.Item>
          {summary.release && <DescriptionList.Item label="Release">{summary.release}</DescriptionList.Item>}
        </DescriptionList>
      </Section>
    </div>
  );
}
