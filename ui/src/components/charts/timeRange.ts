import type { ChartTimeRange, TimeRangePreset } from './types';

/** Presets matching Proxmox's RRD timeframes (hour, day, week). */
export const DEFAULT_TIME_RANGE_PRESETS: TimeRangePreset[] = [
  { label: '1h', duration: 60 * 60 * 1000 },
  { label: '24h', duration: 24 * 60 * 60 * 1000 },
  { label: '7d', duration: 7 * 24 * 60 * 60 * 1000 },
];

/** The preset whose duration is closest to the range's span. */
export function matchPreset(range: ChartTimeRange, presets: readonly TimeRangePreset[]): TimeRangePreset {
  const duration = range.to.getTime() - range.from.getTime();
  let closest = presets[0];
  for (const p of presets) {
    if (Math.abs(duration - p.duration) < Math.abs(duration - closest.duration)) closest = p;
  }
  return closest;
}

/** A range ending now covering `duration` ms. */
export function rangeFromDuration(duration: number, now = Date.now()): ChartTimeRange {
  return { from: new Date(now - duration), to: new Date(now) };
}
