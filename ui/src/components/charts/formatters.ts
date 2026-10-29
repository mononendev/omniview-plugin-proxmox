import type { MetricFormat, ValueFormatter } from './types';

const numberFmt = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 });
const integerFmt = new Intl.NumberFormat(undefined);
const timeFmt = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' });
const dayFmt = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' });
const dateTimeFmt = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
});

/** Format bytes with auto-scaling (B -> TiB). `unit` replaces the scaled unit label. */
export function formatBytes(n: number, unit?: string): string {
  const abs = Math.abs(n);
  if (abs < 1024) return `${numberFmt.format(n)} ${unit ?? 'B'}`;
  if (abs < 1024 ** 2) return `${numberFmt.format(n / 1024)} ${unit ?? 'KiB'}`;
  if (abs < 1024 ** 3) return `${numberFmt.format(n / 1024 ** 2)} ${unit ?? 'MiB'}`;
  if (abs < 1024 ** 4) return `${numberFmt.format(n / 1024 ** 3)} ${unit ?? 'GiB'}`;
  return `${numberFmt.format(n / 1024 ** 4)} ${unit ?? 'TiB'}`;
}

/** Format a value as a percentage. */
export function formatPercent(v: number, unit?: string): string {
  return `${numberFmt.format(v)}${unit ?? '%'}`;
}

/** Format a duration in ms with auto-scaling. */
export function formatDuration(ms: number, unit?: string): string {
  const abs = Math.abs(ms);
  if (unit) return `${numberFmt.format(ms)} ${unit}`;
  if (abs < 1000) return `${numberFmt.format(ms)} ms`;
  if (abs < 60_000) return `${numberFmt.format(ms / 1000)} s`;
  if (abs < 3_600_000) return `${numberFmt.format(ms / 60_000)} m`;
  return `${numberFmt.format(ms / 3_600_000)} h`;
}

const siPrefixes = ['', 'k', 'M', 'G', 'T', 'P'];

function scaleSI(v: number): [string, string] {
  let idx = 0;
  let scaled = Math.abs(v);
  while (scaled >= 1000 && idx < siPrefixes.length - 1) {
    scaled /= 1000;
    idx++;
  }
  return [`${v < 0 ? '-' : ''}${numberFmt.format(scaled)}`, siPrefixes[idx]];
}

/** Format a rate with SI prefix (default unit `/s`). */
export function formatRate(v: number, unit?: string): string {
  const [num, prefix] = scaleSI(v);
  return `${num} ${prefix}${unit ?? '/s'}`;
}

/** Format with SI prefix (1.2 k, 3.4 M). */
export function formatSI(v: number, unit?: string): string {
  const [num, prefix] = scaleSI(v);
  const suffix = unit ? ` ${unit}` : prefix ? ` ${prefix}` : '';
  return `${num}${suffix}`;
}

/** Locale-aware number. */
export function formatNumber(v: number, unit?: string): string {
  const formatted = integerFmt.format(v);
  return unit ? `${formatted} ${unit}` : formatted;
}

/** Returns a null-safe formatter for a MetricFormat preset. */
export function getValueFormatter(format: MetricFormat, unit?: string): ValueFormatter {
  const inner = ((): ((v: number) => string) => {
    switch (format) {
      case 'bytes':
        return (v) => formatBytes(v, unit);
      case 'percent':
        return (v) => formatPercent(v, unit);
      case 'duration':
        return (v) => formatDuration(v, unit);
      case 'rate':
        return (v) => formatRate(v, unit);
      case 'si':
        return (v) => formatSI(v, unit);
      case 'number':
      default:
        return (v) => formatNumber(v, unit);
    }
  })();
  return (v) => (v == null || Number.isNaN(v) ? '–' : inner(v));
}

/** Axis tick label for a timestamp; shows the date when the visible span exceeds two days. */
export function formatTimeTick(ms: number, spanMs: number): string {
  return spanMs > 2 * 86_400_000 ? dayFmt.format(ms) : timeFmt.format(ms);
}

/** Full timestamp for hover readouts. */
export function formatTimestamp(ms: number): string {
  return dateTimeFmt.format(ms);
}
