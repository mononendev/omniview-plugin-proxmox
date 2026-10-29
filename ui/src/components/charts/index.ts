/*
 * SVG time-series charts, copied from the kubernetes plugin's
 * components/shared/charts: neither the runtime nor base-ui exports charts.
 */
export type { TimeSeriesPoint, TimeSeriesDef, MetricFormat, ChartTimeRange, TimeRangePreset } from './types';
export { DEFAULT_TIME_RANGE_PRESETS, rangeFromDuration } from './timeRange';
export { default as MetricsPanel } from './MetricsPanel';
