import { useEffect, useMemo, useState } from 'react';
import { EmptyState, Meter, SegmentedControl, Spinner, Stack, Text } from '@omniviewdev/base-ui';
import { type HostRecord, useResourceMetrics } from '@omniviewdev/runtime';
import { MetricShape, type QueryResponse } from '@omniviewdev/runtime/models';
import { LuChartLine } from 'react-icons/lu';

import { PLUGIN_ID, RESOURCE_KEYS, USAGE_ZONES } from '../constants';
import { formatBytes } from '../utils/columns';
import {
  type ChartTimeRange,
  DEFAULT_TIME_RANGE_PRESETS,
  type TimeSeriesDef,
  MetricsPanel,
  rangeFromDuration,
} from './charts';

const rootStyle = { width: '100%' } as const;
const centerStyle = { display: 'flex', justifyContent: 'center', padding: '32px 0' } as const;
const tileGridStyle = { display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 8 } as const;
const tileStyle = {
  display: 'flex',
  flexDirection: 'column',
  gap: 4,
  padding: '6px 8px',
  borderRadius: 4,
  border: '1px solid var(--ov-color-border-default)',
  background: 'var(--ov-color-bg-surface-raised)',
  minWidth: 0,
} as const;
const tileValueStyle = { fontVariantNumeric: 'tabular-nums' } as const;

/** The chart window slides forward this often. */
const REFRESH_MS = 60_000;
const CHART_HEIGHT = 160;

/** Metric IDs the backend serves (see the plugin's metric provider). */
const COMMON_METRICS = ['cpu', 'memory', 'memory_total', 'net_in', 'net_out'];
const GUEST_METRICS = [...COMMON_METRICS, 'disk_read', 'disk_write'];
const NODE_METRICS = [...COMMON_METRICS, 'load', 'iowait', 'rootfs', 'rootfs_total'];

const formatRate = (v: number | null) => (v == null || Number.isNaN(v) ? '–' : `${formatBytes(v)}/s`);
const formatSize = (v: number | null) => (v == null || Number.isNaN(v) ? '–' : formatBytes(v));
const formatLoad = (v: number | null) => (v == null || Number.isNaN(v) ? '–' : v.toFixed(2));

type Props = {
  /** The host's current record of the node or guest. */
  record: HostRecord;
};

type Extracted = {
  current: Map<string, number>;
  series: Map<string, TimeSeriesDef['data']>;
  /** First provider error, e.g. a missing Sys.Audit privilege. */
  error?: string;
};

function extract(responses: Record<string, QueryResponse> | undefined): Extracted {
  const out: Extracted = { current: new Map(), series: new Map() };
  for (const resp of Object.values(responses ?? {})) {
    if (!resp?.success) {
      out.error ??= resp?.error || undefined;
      continue;
    }
    for (const result of resp.results ?? []) {
      if (result.current_value) out.current.set(result.current_value.metric_id, result.current_value.value);
      if (result.time_series) {
        out.series.set(
          result.time_series.metric_id,
          (result.time_series.data_points ?? []).map((dp) => ({
            timestamp: new Date(dp.timestamp).getTime(),
            value: dp.value,
          })),
        );
      }
    }
  }
  return out;
}

/**
 * Drawer view of a node's or guest's RRD metrics: current CPU and memory
 * use, then time series over the last hour, day or week.
 */
export default function ResourceMetrics({ record }: Props) {
  const { reference } = record.record;
  const isNode = reference.address.type === RESOURCE_KEYS.node;
  const metricIDs = isNode ? NODE_METRICS : GUEST_METRICS;
  const [duration, setDuration] = useState(DEFAULT_TIME_RANGE_PRESETS[0].duration);
  const [range, setRange] = useState<ChartTimeRange>(() => rangeFromDuration(duration));

  useEffect(() => {
    setRange(rangeFromDuration(duration));
    const timer = setInterval(() => setRange(rangeFromDuration(duration)), REFRESH_MS);
    return () => clearInterval(timer);
  }, [duration]);

  const query = {
    pluginID: PLUGIN_ID,
    connectionID: record.route.connection,
    resourceKey: reference.address.type,
    resourceID: reference.address.id,
    resourceData: record.record.body,
    reference,
    route: record.route,
    metricIDs,
  };
  const current = useResourceMetrics({ ...query, shape: MetricShape.ShapeCurrent, refreshInterval: REFRESH_MS });
  const timeseries = useResourceMetrics({
    ...query,
    shape: MetricShape.ShapeTimeseries,
    timeRange: { start: range.from, end: range.to, step: '' },
  });

  const now = useMemo(() => extract(current.data), [current.data]);
  const history = useMemo(() => extract(timeseries.data), [timeseries.data]);

  if (current.isLoading && now.current.size === 0) {
    return (
      <div style={centerStyle}>
        <Spinner size="sm" />
      </div>
    );
  }
  if (current.providers.length === 0) {
    return <EmptyState size="sm" icon={<LuChartLine />} title="No metrics provider for this resource" />;
  }
  const error = current.error?.message ?? now.error;
  if (error && now.current.size === 0) {
    return <EmptyState size="sm" icon={<LuChartLine />} title="Could not load metrics" description={error} />;
  }

  const series = (defs: Array<[id: string, label: string, color: string, dashed?: boolean]>): TimeSeriesDef[] =>
    defs.flatMap(([id, label, color, dashed]) => {
      const points = history.series.get(id);
      return points ? [{ id, label, data: points, color, lineStyle: dashed ? 'dashed' : 'solid' }] : [];
    });
  const chartError = timeseries.error?.message ?? history.error;
  const panel = { timeRange: range, height: CHART_HEIGHT, loading: timeseries.isLoading, error: chartError, area: true };

  const cpu = now.current.get('cpu');
  const mem = now.current.get('memory');
  const memTotal = now.current.get('memory_total');
  const rootfs = now.current.get('rootfs');
  const rootfsTotal = now.current.get('rootfs_total');

  return (
    <Stack direction="column" spacing={3} style={rootStyle}>
      <div style={tileGridStyle}>
        <UsageTile label="CPU" percent={cpu} detail={cpu === undefined ? undefined : `${cpu.toFixed(1)} %`} />
        <UsageTile
          label="Memory"
          percent={mem !== undefined && memTotal ? (mem / memTotal) * 100 : undefined}
          detail={mem === undefined ? undefined : `${formatBytes(mem)}${memTotal ? ` / ${formatBytes(memTotal)}` : ''}`}
        />
        {isNode ? (
          <>
            <UsageTile
              label="Root disk"
              percent={rootfs !== undefined && rootfsTotal ? (rootfs / rootfsTotal) * 100 : undefined}
              detail={
                rootfs === undefined ? undefined : `${formatBytes(rootfs)}${rootfsTotal ? ` / ${formatBytes(rootfsTotal)}` : ''}`
              }
            />
            <ValueTile
              label="Load · IO wait"
              value={`${formatLoad(now.current.get('load') ?? null)} · ${(now.current.get('iowait') ?? 0).toFixed(1)} %`}
            />
          </>
        ) : (
          <>
            <ValueTile
              label="Network in / out"
              value={`${formatRate(now.current.get('net_in') ?? null)} / ${formatRate(now.current.get('net_out') ?? null)}`}
            />
            <ValueTile
              label="Disk read / write"
              value={`${formatRate(now.current.get('disk_read') ?? null)} / ${formatRate(
                now.current.get('disk_write') ?? null,
              )}`}
            />
          </>
        )}
      </div>

      <Stack direction="row" align="center" justify="between">
        <Text weight="semibold" size="sm">
          History
        </Text>
        <SegmentedControl
          size="xs"
          value={String(duration)}
          onValueChange={(value: string) => setDuration(Number(value))}
          aria-label="Time range"
        >
          {DEFAULT_TIME_RANGE_PRESETS.map((p) => (
            <SegmentedControl.Item key={p.duration} value={String(p.duration)}>
              {p.label}
            </SegmentedControl.Item>
          ))}
        </SegmentedControl>
      </Stack>

      <MetricsPanel
        {...panel}
        title="CPU"
        valueFormat="percent"
        series={series(
          isNode
            ? [
                ['cpu', 'CPU', 'var(--ov-color-brand-500)'],
                ['iowait', 'IO wait', 'var(--ov-color-warning)'],
              ]
            : [['cpu', 'CPU', 'var(--ov-color-brand-500)']],
        )}
      />
      <MetricsPanel
        {...panel}
        title="Memory"
        valueFormatter={formatSize}
        series={series([
          ['memory', 'Used', 'var(--ov-color-brand-500)'],
          ['memory_total', 'Total', 'var(--ov-color-fg-muted)', true],
        ])}
      />
      <MetricsPanel
        {...panel}
        title="Network"
        valueFormatter={formatRate}
        series={series([
          ['net_in', 'In', 'var(--ov-color-success)'],
          ['net_out', 'Out', 'var(--ov-color-warning)'],
        ])}
      />
      {isNode ? (
        <MetricsPanel
          {...panel}
          title="Load average"
          valueFormatter={formatLoad}
          series={series([['load', 'Load', 'var(--ov-color-info)']])}
        />
      ) : (
        <MetricsPanel
          {...panel}
          title="Disk I/O"
          valueFormatter={formatRate}
          series={series([
            ['disk_read', 'Read', 'var(--ov-color-success)'],
            ['disk_write', 'Write', 'var(--ov-color-warning)'],
          ])}
        />
      )}
    </Stack>
  );
}

function UsageTile({ label, percent, detail }: { label: string; percent?: number; detail?: string }) {
  return (
    <div style={tileStyle}>
      <Stack direction="row" justify="between" align="baseline">
        <Text size="xs" tone="muted">
          {label}
        </Text>
        <Text size="xs" weight="semibold" style={tileValueStyle}>
          {detail ?? '—'}
        </Text>
      </Stack>
      <Meter
        size="sm"
        value={percent === undefined ? 0 : Math.min(percent, 100)}
        {...USAGE_ZONES}
        aria-label={`${label} ${percent === undefined ? 'unknown' : `${percent.toFixed(0)}%`}`}
      />
    </div>
  );
}

function ValueTile({ label, value }: { label: string; value: string }) {
  return (
    <div style={tileStyle}>
      <Text size="xs" tone="muted">
        {label}
      </Text>
      <Text size="sm" weight="semibold" style={tileValueStyle}>
        {value}
      </Text>
    </div>
  );
}
