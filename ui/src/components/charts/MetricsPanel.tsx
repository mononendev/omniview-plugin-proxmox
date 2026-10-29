import { SegmentedControl } from '@omniviewdev/base-ui';
import React, { useCallback, useMemo } from 'react';

import styles from './Charts.module.css';
import { DEFAULT_TIME_RANGE_PRESETS, matchPreset, rangeFromDuration } from './timeRange';
import TimeSeriesChart from './TimeSeriesChart';
import type { ChartMargin } from './TimeSeriesChart';
import type {
  ChartAnnotation,
  ChartTimeRange,
  MetricFormat,
  TimeRangePreset,
  TimeSeriesDef,
  ValueFormatter,
} from './types';

export interface MetricsPanelProps {
  title: string;
  /** Small icon rendered before the title. */
  icon?: React.ReactNode;
  /** Muted subtitle below the title. */
  subtitle?: string;
  series: TimeSeriesDef[];
  timeRange: ChartTimeRange;
  /** When set, a preset picker is shown in the header. */
  onTimeRangeChange?: (range: ChartTimeRange) => void;
  presets?: TimeRangePreset[];
  valueFormat?: MetricFormat;
  unit?: string;
  valueFormatter?: ValueFormatter;
  annotations?: ChartAnnotation[];
  area?: boolean;
  colors?: string[];
  /** Extra controls rendered at the right of the header. */
  toolbar?: React.ReactNode;
  /** Time tick labels under the plot. Default 'none' (hover shows the time). */
  xAxisPosition?: 'bottom' | 'none';
  /** Default 'right' (labels drawn inside the plot). */
  yAxis?: 'right' | 'left' | 'outside' | 'none';
  margin?: ChartMargin;
  /** 'compact' = transparent, no border. */
  variant?: 'default' | 'compact';
  height?: number;
  loading?: boolean;
  error?: string;
  className?: string;
  style?: React.CSSProperties;
}

/** Titled time-series chart card with an optional time-range preset picker. */
const MetricsPanel: React.FC<MetricsPanelProps> = ({
  title,
  icon,
  subtitle,
  series,
  timeRange,
  onTimeRangeChange,
  presets = DEFAULT_TIME_RANGE_PRESETS,
  valueFormat = 'number',
  unit,
  valueFormatter,
  annotations,
  area = false,
  colors,
  toolbar,
  xAxisPosition = 'none',
  yAxis = 'right',
  margin,
  variant = 'default',
  height = 260,
  loading,
  error,
  className,
  style,
}) => {
  const selected = useMemo(() => String(matchPreset(timeRange, presets).duration), [timeRange, presets]);

  const handlePreset = useCallback(
    (value: string) => onTimeRangeChange?.(rangeFromDuration(Number(value))),
    [onTimeRangeChange],
  );

  const rootClass = [styles.panel, variant === 'default' ? styles.panelDefault : undefined, className]
    .filter(Boolean)
    .join(' ');

  return (
    <div className={rootClass} style={style}>
      <div className={styles.panelHeader}>
        {icon && <span className={styles.panelIcon}>{icon}</span>}
        <div className={styles.panelTitles}>
          <span className={styles.panelTitle}>{title}</span>
          {subtitle && <span className={styles.panelSubtitle}>{subtitle}</span>}
        </div>
        {(onTimeRangeChange || toolbar) && (
          <div className={styles.panelControls}>
            {onTimeRangeChange && (
              <SegmentedControl size="xs" value={selected} onValueChange={handlePreset} aria-label="Time range">
                {presets.map((p) => (
                  <SegmentedControl.Item key={p.duration} value={String(p.duration)}>
                    {p.label}
                  </SegmentedControl.Item>
                ))}
              </SegmentedControl>
            )}
            {toolbar}
          </div>
        )}
      </div>
      <TimeSeriesChart
        series={series}
        timeRange={timeRange}
        valueFormat={valueFormat}
        unit={unit}
        valueFormatter={valueFormatter}
        annotations={annotations}
        area={area}
        colors={colors}
        height={height}
        loading={loading}
        error={error}
        xAxisPosition={xAxisPosition}
        yAxis={yAxis}
        margin={margin}
        compact
        grid
      />
    </div>
  );
};

MetricsPanel.displayName = 'MetricsPanel';

export default MetricsPanel;
