import { Skeleton, Text } from '@omniviewdev/base-ui';
import React, { useCallback, useId, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { LuChartBar } from 'react-icons/lu';

import styles from './Charts.module.css';
import { formatTimeTick, formatTimestamp, getValueFormatter } from './formatters';
import {
  alignSeries,
  buildAreaPath,
  buildLinePath,
  linearScale,
  nearestIndex,
  niceTicks,
  resolveChartColor,
  seriesColor,
  timeTicks,
  valueExtent,
} from './scales';
import type {
  ChartAnnotation,
  ChartStateProps,
  ChartTimeRange,
  MetricFormat,
  TimeSeriesDef,
  ValueFormatter,
} from './types';

export interface ChartMargin {
  top?: number;
  right?: number;
  bottom?: number;
  left?: number;
}

export interface TimeSeriesChartProps extends ChartStateProps {
  series: TimeSeriesDef[];
  /** Visible x-domain. Defaults to the data extent. */
  timeRange?: ChartTimeRange;
  valueFormat?: MetricFormat;
  /** Unit suffix passed to the `valueFormat` preset. */
  unit?: string;
  /** Overrides valueFormat + unit. */
  valueFormatter?: ValueFormatter;
  annotations?: ChartAnnotation[];
  /** Fill the area under every series (per-series `area` wins). */
  area?: boolean;
  /** Draw horizontal grid lines. Default true. */
  grid?: boolean;
  /** Custom palette for series without an explicit colour. */
  colors?: string[];
  /** Show time tick labels under the plot. Default 'bottom'. */
  xAxisPosition?: 'bottom' | 'none';
  /** Show y tick labels inside the plot ('right'/'left'), or outside on the left ('outside'), or not at all. Default 'right'. */
  yAxis?: 'right' | 'left' | 'outside' | 'none';
  /** Compact mode trims outer margins. */
  compact?: boolean;
  margin?: ChartMargin;
  height?: number;
  className?: string;
  style?: React.CSSProperties;
}

const DEFAULT_MARGIN: Required<ChartMargin> = { top: 8, right: 8, bottom: 8, left: 8 };
const COMPACT_MARGIN: Required<ChartMargin> = { top: 4, right: 2, bottom: 2, left: 2 };
const X_AXIS_HEIGHT = 18;
const OUTSIDE_Y_WIDTH = 44;
const AREA_OPACITY = 0.15;
const DASH: Record<string, string | undefined> = { dashed: '6 3', dotted: '2 2', solid: undefined };

/** Measure an element's content width, updating on resize. */
function useElementWidth<T extends HTMLElement>(): [React.RefObject<T | null>, number] {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    setWidth(el.getBoundingClientRect().width);
    if (typeof ResizeObserver === 'undefined') return;
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width;
      if (w != null) setWidth((prev) => (Math.abs(prev - w) < 0.5 ? prev : w));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, width];
}

function ChartState({ height, children }: { height: number; children: React.ReactNode }) {
  return (
    <div className={styles.state} style={{ height }}>
      {children}
    </div>
  );
}

const TimeSeriesChart: React.FC<TimeSeriesChartProps> = ({
  series,
  timeRange,
  valueFormat = 'number',
  unit,
  valueFormatter,
  annotations,
  area = false,
  grid = true,
  colors,
  xAxisPosition = 'bottom',
  yAxis = 'right',
  compact = false,
  margin: marginOverride,
  height = 260,
  loading,
  error,
  emptyContent,
  className,
  style,
}) => {
  const clipId = useId();
  const [rootRef, width] = useElementWidth<HTMLDivElement>();
  const [hoverIdx, setHoverIdx] = useState<number | null>(null);

  const fmt = useMemo(
    () => valueFormatter ?? getValueFormatter(valueFormat, unit),
    [valueFormatter, valueFormat, unit],
  );

  const aligned = useMemo(() => alignSeries(series), [series]);
  const isEmpty = aligned.timestamps.length === 0;

  const margin = { ...(compact ? COMPACT_MARGIN : DEFAULT_MARGIN), ...marginOverride };
  const plotLeft = margin.left + (yAxis === 'outside' ? OUTSIDE_Y_WIDTH : 0);
  const plotTop = margin.top;
  const plotRight = Math.max(plotLeft + 1, width - margin.right);
  const plotBottom = Math.max(
    plotTop + 1,
    height - margin.bottom - (xAxisPosition === 'bottom' ? X_AXIS_HEIGHT : 0),
  );

  // Domains and scales.
  const geometry = useMemo(() => {
    const { timestamps, values } = aligned;
    const x0 = timeRange ? timeRange.from.getTime() : (timestamps[0] ?? 0);
    const x1 = timeRange ? timeRange.to.getTime() : (timestamps[timestamps.length - 1] ?? 1);
    const ext = valueExtent(values) ?? [0, 1];
    const y = niceTicks(Math.min(0, ext[0]), ext[1], Math.max(2, Math.round((plotBottom - plotTop) / 40)));
    const xScale = linearScale([x0, x1 === x0 ? x0 + 1 : x1], [plotLeft, plotRight]);
    const yScale = linearScale([y.min, y.max], [plotBottom, plotTop]);
    const xs = timestamps.map(xScale);
    const baseline = yScale(Math.max(y.min, 0));
    const paths = series.map((s, i) => {
      const ys = values[i].map((v) => (v == null ? null : yScale(v)));
      return {
        line: buildLinePath(xs, ys),
        area: (s.area ?? area) ? buildAreaPath(xs, ys, baseline) : '',
        color: seriesColor(i, s.color, colors),
        dash: DASH[s.lineStyle ?? 'solid'],
      };
    });
    const xTicks = timeTicks(x0, x1, Math.max(2, Math.floor((plotRight - plotLeft) / 90)));
    return { x0, x1, xs, xScale, yScale, yTicks: y.ticks, paths, xTicks };
  }, [aligned, series, area, colors, timeRange, plotLeft, plotRight, plotTop, plotBottom]);

  const handlePointerMove = useCallback(
    (e: React.PointerEvent<SVGRectElement>) => {
      const rect = e.currentTarget.getBoundingClientRect();
      const px = e.clientX - rect.left + plotLeft;
      const { x0, x1 } = geometry;
      const t = x0 + ((px - plotLeft) / Math.max(1, plotRight - plotLeft)) * (x1 - x0);
      const idx = nearestIndex(aligned.timestamps, t);
      setHoverIdx(idx < 0 ? null : idx);
    },
    [geometry, aligned.timestamps, plotLeft, plotRight],
  );
  const handlePointerLeave = useCallback(() => setHoverIdx(null), []);

  let body: React.ReactNode;
  if (loading) {
    body = <Skeleton variant="rectangular" width="100%" height={height} />;
  } else if (error) {
    body = (
      <ChartState height={height}>
        <Text size="sm" tone="danger">
          {error}
        </Text>
      </ChartState>
    );
  } else if (isEmpty) {
    body = (
      <ChartState height={height}>
        {emptyContent ?? (
          <>
            <LuChartBar size={24} />
            <Text size="sm" weight="medium">
              No data
            </Text>
            <Text size="xs" tone="muted">
              There is no data to display.
            </Text>
          </>
        )}
      </ChartState>
    );
  } else if (width > 0) {
    const { xs, yScale, yTicks, paths, xTicks, xScale, x1, x0 } = geometry;
    const hoverX = hoverIdx != null ? xs[hoverIdx] : null;
    const inlineRight = yAxis === 'right';
    body = (
      <>
        <svg className={styles.svg} width={width} height={height} role="img" aria-label="Time series chart">
          <defs>
            <clipPath id={clipId}>
              <rect x={plotLeft} y={plotTop - 2} width={plotRight - plotLeft} height={plotBottom - plotTop + 4} />
            </clipPath>
          </defs>

          {grid &&
            yTicks.map((tick) => {
              const y = yScale(tick);
              return (
                <line key={`g${tick}`} className={styles.gridLine} x1={plotLeft} x2={plotRight} y1={y} y2={y} />
              );
            })}

          <g clipPath={`url(#${clipId})`}>
            {paths.map((p, i) =>
              p.area ? (
                <path key={`a${series[i].id}`} d={p.area} style={{ fill: p.color, fillOpacity: AREA_OPACITY, stroke: 'none' }} />
              ) : null,
            )}
            {paths.map((p, i) => (
              <path
                key={`l${series[i].id}`}
                d={p.line}
                style={{ fill: 'none', stroke: p.color }}
                strokeWidth={1.5}
                strokeDasharray={p.dash}
                strokeLinejoin="round"
                strokeLinecap="round"
              />
            ))}
            {annotations?.map((ann, i) => {
              const y = yScale(ann.value);
              if (y < plotTop - 1 || y > plotBottom + 1) return null;
              return (
                <g key={`ann${i}`}>
                  <line
                    x1={plotLeft}
                    x2={plotRight}
                    y1={y}
                    y2={y}
                    style={{ stroke: ann.color ? resolveChartColor(ann.color) : 'var(--ov-color-fg-muted)' }}
                    strokeDasharray={ann.lineStyle === 'dashed' ? '6 3' : undefined}
                  />
                  {ann.label && (
                    <text className={styles.annotationLabel} x={plotLeft + 4} y={y - 3}>
                      {ann.label}
                    </text>
                  )}
                </g>
              );
            })}
          </g>

          {yAxis !== 'none' &&
            yTicks.map((tick) => {
              const y = yScale(tick);
              if (yAxis === 'outside') {
                return (
                  <text key={`y${tick}`} className={styles.axisLabel} x={plotLeft - 4} y={y} textAnchor="end" dominantBaseline="middle">
                    {fmt(tick)}
                  </text>
                );
              }
              return (
                <text
                  key={`y${tick}`}
                  className={styles.inlineAxisLabel}
                  x={inlineRight ? plotRight - 4 : plotLeft + 4}
                  y={y - 4}
                  textAnchor={inlineRight ? 'end' : 'start'}
                >
                  {fmt(tick)}
                </text>
              );
            })}

          {xAxisPosition === 'bottom' &&
            xTicks.map((t) => (
              <text
                key={`x${t}`}
                className={styles.axisLabel}
                x={xScale(t)}
                y={plotBottom + 13}
                textAnchor="middle"
              >
                {formatTimeTick(t, x1 - x0)}
              </text>
            ))}

          {hoverX != null && (
            <g>
              <line className={styles.crosshair} x1={hoverX} x2={hoverX} y1={plotTop} y2={plotBottom} />
              {paths.map((p, i) => {
                const v = aligned.values[i][hoverIdx!];
                if (v == null) return null;
                return <circle key={`d${series[i].id}`} className={styles.hoverDot} cx={hoverX} cy={yScale(v)} r={3} style={{ fill: p.color }} />;
              })}
            </g>
          )}

          <rect
            x={plotLeft}
            y={plotTop}
            width={plotRight - plotLeft}
            height={plotBottom - plotTop}
            fill="transparent"
            onPointerMove={handlePointerMove}
            onPointerLeave={handlePointerLeave}
          />
        </svg>

        {hoverX != null && (
          <div
            className={styles.readout}
            style={hoverX > width / 2 ? { right: width - hoverX + 10 } : { left: hoverX + 10 }}
          >
            <div className={styles.readoutTime}>{formatTimestamp(aligned.timestamps[hoverIdx!])}</div>
            {series.map((s, i) => (
              <div key={s.id} className={styles.readoutRow}>
                <span className={styles.swatch} style={{ background: paths[i].color }} />
                <span className={styles.readoutLabel}>{s.label}</span>
                <span className={styles.readoutValue}>{fmt(aligned.values[i][hoverIdx!])}</span>
              </div>
            ))}
          </div>
        )}
      </>
    );
  }

  return (
    <div ref={rootRef} className={[styles.chartRoot, className].filter(Boolean).join(' ')} style={{ height, ...style }}>
      {body}
    </div>
  );
};

TimeSeriesChart.displayName = 'TimeSeriesChart';

export default TimeSeriesChart;
