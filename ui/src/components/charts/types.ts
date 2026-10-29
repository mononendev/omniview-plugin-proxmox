import type { ReactNode } from 'react';

/** A time-stamped data point. */
export interface TimeSeriesPoint {
  /** Epoch milliseconds. */
  timestamp: number;
  value: number;
}

/** A named series for a time-series chart. */
export interface TimeSeriesDef {
  id: string;
  label: string;
  data: TimeSeriesPoint[];
  /** CSS colour (tokens like `var(--ov-color-success)` work) or a semantic name (see `resolveChartColor`). */
  color?: string;
  area?: boolean;
  lineStyle?: 'solid' | 'dashed' | 'dotted';
}

/** Horizontal reference/threshold line. */
export interface ChartAnnotation {
  value: number;
  label?: string;
  color?: string;
  lineStyle?: 'solid' | 'dashed';
}

/** Built-in value format presets. */
export type MetricFormat = 'bytes' | 'percent' | 'duration' | 'rate' | 'si' | 'number';

/** Absolute time range shown by a chart. */
export interface ChartTimeRange {
  from: Date;
  to: Date;
}

/** A relative time-range preset ("last 1h"). */
export interface TimeRangePreset {
  label: string;
  /** Milliseconds. */
  duration: number;
}

export type ValueFormatter = (value: number | null) => string;

export interface ChartStateProps {
  loading?: boolean;
  error?: string;
  /** Rendered instead of the default "No data" placeholder. */
  emptyContent?: ReactNode;
}
