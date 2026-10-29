import type { TimeSeriesDef } from './types';

/** Semantic colour names accepted in addition to raw CSS colours. */
const SEMANTIC_COLORS: Record<string, string> = {
  primary: 'var(--ov-color-brand-500)',
  brand: 'var(--ov-color-brand-500)',
  accent: 'var(--ov-color-brand-500)',
  secondary: 'var(--ov-color-secondary)',
  success: 'var(--ov-color-success)',
  warning: 'var(--ov-color-warning)',
  error: 'var(--ov-color-danger)',
  danger: 'var(--ov-color-danger)',
  info: 'var(--ov-color-info)',
  discovery: 'var(--ov-color-discovery)',
  neutral: 'var(--ov-color-fg-default)',
  muted: 'var(--ov-color-fg-muted)',
};

/** Default ordered palette for chart series (token based, theme aware). */
export const CHART_PALETTE: readonly string[] = [
  'var(--ov-color-brand-500)',
  'var(--ov-color-success)',
  'var(--ov-color-warning)',
  'var(--ov-color-info)',
  'var(--ov-color-danger)',
  'var(--ov-color-secondary)',
  'var(--ov-color-discovery)',
];

/** Resolve a semantic colour name to a token; any other string is returned unchanged. */
export function resolveChartColor(color: string): string {
  return SEMANTIC_COLORS[color] ?? color;
}

/** Series colour: explicit colour, then custom palette, then default palette. */
export function seriesColor(index: number, explicit?: string, palette?: readonly string[]): string {
  if (explicit) return resolveChartColor(explicit);
  const source = palette && palette.length > 0 ? palette : CHART_PALETTE;
  return resolveChartColor(source[index % source.length]);
}

/** Linear mapping from a domain to a range. Degenerate domains map to the range start. */
export function linearScale(
  [d0, d1]: readonly [number, number],
  [r0, r1]: readonly [number, number],
): (v: number) => number {
  const span = d1 - d0;
  if (span === 0) return () => r0;
  const k = (r1 - r0) / span;
  return (v) => r0 + (v - d0) * k;
}

function niceStep(rawStep: number): number {
  const exp = Math.floor(Math.log10(rawStep));
  const base = 10 ** exp;
  const f = rawStep / base;
  const nice = f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10;
  return nice * base;
}

/**
 * Round a [min, max] extent outward to "nice" step boundaries and return the ticks.
 * A flat extent is widened so a chart always has a visible range.
 */
export function niceTicks(
  min: number,
  max: number,
  count = 4,
): { min: number; max: number; ticks: number[] } {
  let lo = Number.isFinite(min) ? min : 0;
  let hi = Number.isFinite(max) ? max : 1;
  if (hi < lo) [lo, hi] = [hi, lo];
  if (hi === lo) {
    hi = lo === 0 ? 1 : lo + Math.abs(lo) * 0.5;
    if (lo > 0) lo = 0;
  }
  const step = niceStep((hi - lo) / Math.max(1, count));
  const niceMin = Math.floor(lo / step) * step;
  const niceMax = Math.ceil(hi / step) * step;
  const ticks: number[] = [];
  // Use integer multiples to avoid floating-point drift.
  const n = Math.round((niceMax - niceMin) / step);
  for (let i = 0; i <= n; i++) ticks.push(Number((niceMin + i * step).toPrecision(12)));
  return { min: niceMin, max: niceMax, ticks };
}

const TIME_STEPS = [
  1_000, 5_000, 15_000, 30_000, 60_000, 2 * 60_000, 5 * 60_000, 10 * 60_000, 15 * 60_000,
  30 * 60_000, 3_600_000, 2 * 3_600_000, 3 * 3_600_000, 6 * 3_600_000, 12 * 3_600_000,
  86_400_000, 2 * 86_400_000, 7 * 86_400_000,
];

/** Evenly spaced, step-aligned timestamps inside [from, to] (local-time aligned for >= 1h steps). */
export function timeTicks(from: number, to: number, count = 5): number[] {
  if (!(to > from)) return [];
  const raw = (to - from) / Math.max(1, count);
  const step = TIME_STEPS.find((s) => s >= raw) ?? TIME_STEPS[TIME_STEPS.length - 1];
  const tzOffset = step >= 3_600_000 ? new Date(from).getTimezoneOffset() * 60_000 : 0;
  let t = Math.ceil((from - tzOffset) / step) * step + tzOffset;
  const out: number[] = [];
  while (t <= to && out.length < 100) {
    out.push(t);
    t += step;
  }
  return out;
}

export interface AlignedSeries {
  /** Sorted unique timestamps across every series. */
  timestamps: number[];
  /** One row per series, aligned to `timestamps`; gaps are null. */
  values: (number | null)[][];
}

/**
 * Unify timestamps across series (different queries can return different point counts)
 * and fill gaps with null.
 */
export function alignSeries(series: readonly TimeSeriesDef[]): AlignedSeries {
  const tsSet = new Set<number>();
  for (const s of series) for (const p of s.data) tsSet.add(p.timestamp);
  const timestamps = Array.from(tsSet).sort((a, b) => a - b);
  const index = new Map<number, number>();
  timestamps.forEach((t, i) => index.set(t, i));
  const values = series.map((s) => {
    const row = Array.from<number | null>({ length: timestamps.length }).fill(null);
    for (const p of s.data) {
      if (Number.isFinite(p.value)) row[index.get(p.timestamp)!] = p.value;
    }
    return row;
  });
  return { timestamps, values };
}

/** Min/max of all non-null values (undefined when there are none). */
export function valueExtent(rows: readonly (readonly (number | null)[])[]): [number, number] | undefined {
  let min = Infinity;
  let max = -Infinity;
  for (const row of rows) {
    for (const v of row) {
      if (v == null) continue;
      if (v < min) min = v;
      if (v > max) max = v;
    }
  }
  return min === Infinity ? undefined : [min, max];
}

function fmt(n: number): string {
  return (Math.round(n * 100) / 100).toString();
}

/** SVG path for a line; null values break the line into separate segments. */
export function buildLinePath(xs: readonly number[], ys: readonly (number | null)[]): string {
  let d = '';
  let pen = false;
  for (let i = 0; i < xs.length; i++) {
    const y = ys[i];
    if (y == null) {
      pen = false;
      continue;
    }
    d += `${pen ? 'L' : 'M'}${fmt(xs[i])},${fmt(y)}`;
    pen = true;
  }
  return d;
}

/** SVG path for a filled area down to `baseline`; each contiguous run is a closed shape. */
export function buildAreaPath(
  xs: readonly number[],
  ys: readonly (number | null)[],
  baseline: number,
): string {
  let d = '';
  let start = -1;
  const close = (end: number) => {
    if (start < 0) return;
    d += `M${fmt(xs[start])},${fmt(baseline)}`;
    for (let j = start; j <= end; j++) d += `L${fmt(xs[j])},${fmt(ys[j] as number)}`;
    d += `L${fmt(xs[end])},${fmt(baseline)}Z`;
    start = -1;
  };
  for (let i = 0; i < xs.length; i++) {
    if (ys[i] == null) close(i - 1);
    else if (start < 0) start = i;
  }
  close(xs.length - 1);
  return d;
}

/** Index of the timestamp closest to `t` in a sorted array (-1 when empty). */
export function nearestIndex(sorted: readonly number[], t: number): number {
  if (sorted.length === 0) return -1;
  let lo = 0;
  let hi = sorted.length - 1;
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1;
    if (sorted[mid] <= t) lo = mid;
    else hi = mid;
  }
  return Math.abs(sorted[lo] - t) <= Math.abs(sorted[hi] - t) ? lo : hi;
}
