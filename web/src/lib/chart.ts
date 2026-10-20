// What every chart shares: round ticks, date ticks, a smooth path through
// points, and finding the point under a pointer.

import { dayShort, month, monthYear } from "../format";

export const DAY = 86_400_000;

/** A day of the API (YYYY-MM-DD) as UTC milliseconds. */
export const utc = (d: string) => Date.parse(d.length === 7 ? d + "-01T00:00:00Z" : d + "T00:00:00Z");

/** Round steps from lo to hi: 1, 2, 2.5, 5 × 10ⁿ, covering both. */
export function niceTicks(lo: number, hi: number, count: number): number[] {
  if (!(hi > lo)) {
    const p = Math.abs(lo) || 1;
    return [lo - p * 0.1, lo + p * 0.1];
  }
  const raw = (hi - lo) / count;
  const p = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * p).find((s) => s >= raw) ?? 10 * p;
  const out: number[] = [];
  for (let v = Math.floor(lo / step + 1e-9) * step; v < hi + step * 0.999; v += step) out.push(Math.round(v / step) * step);
  if (out[out.length - 1] < hi) out.push(out[out.length - 1] + step);
  return out;
}

/** Date ticks that suit the span: days, months or years. */
export function timeTicks(t0: number, t1: number, width: number): { t: number; label: string }[] {
  const span = (t1 - t0) / DAY;
  const out: { t: number; label: string }[] = [];
  const d = new Date(t0);
  const room = Math.max(2, Math.floor(width / 90));
  if (span <= 10) {
    for (let t = t0; t <= t1; t += DAY) out.push({ t, label: dayShort(t) });
  } else if (span <= 62) {
    const s = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()));
    while (s.getUTCDay() !== 1) s.setUTCDate(s.getUTCDate() + 1);
    for (; s.getTime() <= t1; s.setUTCDate(s.getUTCDate() + 7)) out.push({ t: s.getTime(), label: dayShort(s.getTime()) });
  } else if (span <= 800) {
    const s = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 1));
    for (; s.getTime() <= t1; s.setUTCMonth(s.getUTCMonth() + 1)) {
      const t = s.getTime();
      out.push({ t, label: s.getUTCMonth() === 0 ? String(s.getUTCFullYear()) : month(t) });
    }
  } else {
    for (let y = d.getUTCFullYear() + 1; Date.UTC(y, 0, 1) <= t1; y++) out.push({ t: Date.UTC(y, 0, 1), label: String(y) });
  }
  if (out.length <= room) return out;
  // Thin evenly, years and first ticks kept first.
  const every = Math.ceil(out.length / room);
  return out.filter((_, n) => n % every === 0).map((k) => (k.label.length > 4 && span > 300 ? { ...k, label: monthYear(k.t) } : k));
}

/**
 * A smooth path through points sorted by x that never overshoots (monotone
 * cubic, as d3's curveMonotoneX).
 */
export function smooth(xs: ArrayLike<number>, ys: ArrayLike<number>): string {
  const n = xs.length;
  if (!n) return "";
  const f = (v: number) => (Math.round(v * 10) / 10).toString();
  if (n < 3) {
    let s = `M${f(xs[0])},${f(ys[0])}`;
    for (let i = 1; i < n; i++) s += `L${f(xs[i])},${f(ys[i])}`;
    return s;
  }
  // Many points for the width: straight segments read as smooth already.
  if (n > 600) {
    let s = `M${f(xs[0])},${f(ys[0])}`;
    for (let i = 1; i < n; i++) s += `L${f(xs[i])},${f(ys[i])}`;
    return s;
  }
  // Tangents as d3's monotoneX: the smaller of the neighbouring slopes,
  // zero at a peak or a trough.
  const m = new Float64Array(n);
  const sl = (i: number) => {
    const h = xs[i + 1] - xs[i];
    return h ? (ys[i + 1] - ys[i]) / h : 0;
  };
  for (let i = 1; i < n - 1; i++) {
    const h0 = xs[i] - xs[i - 1];
    const h1 = xs[i + 1] - xs[i];
    const s0 = sl(i - 1);
    const s1 = sl(i);
    const p = (s0 * h1 + s1 * h0) / (h0 + h1 || 1);
    m[i] = (Math.sign(s0) + Math.sign(s1)) * Math.min(Math.abs(s0), Math.abs(s1), 0.5 * Math.abs(p)) || 0;
  }
  m[0] = (3 * sl(0) - m[1]) / 2;
  m[n - 1] = (3 * sl(n - 2) - m[n - 2]) / 2;
  let s = `M${f(xs[0])},${f(ys[0])}`;
  for (let i = 0; i < n - 1; i++) {
    const h = (xs[i + 1] - xs[i]) / 3;
    s += `C${f(xs[i] + h)},${f(ys[i] + m[i] * h)},${f(xs[i + 1] - h)},${f(ys[i + 1] - m[i + 1] * h)},${f(xs[i + 1])},${f(ys[i + 1])}`;
  }
  return s;
}

/** Index of the value nearest to v in a sorted array. */
export function nearest(sorted: ArrayLike<number>, v: number): number {
  let lo = 0;
  let hi = sorted.length - 1;
  if (hi <= 0) return 0;
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1;
    if (sorted[mid] < v) lo = mid;
    else hi = mid;
  }
  return v - sorted[lo] < sorted[hi] - v ? lo : hi;
}

/** y of a polyline at x (clamped at its ends). */
export function sampleAt(xs: ArrayLike<number>, ys: ArrayLike<number>, x: number): number {
  const n = xs.length;
  if (!n) return 0;
  if (x <= xs[0]) return ys[0];
  if (x >= xs[n - 1]) return ys[n - 1];
  const i = nearest(xs, x);
  const j = xs[i] <= x ? i : i - 1;
  const k = Math.min(n - 1, j + 1);
  const span = xs[k] - xs[j];
  return span ? ys[j] + ((x - xs[j]) / span) * (ys[k] - ys[j]) : ys[j];
}

export const reducedMotion = () => typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;

/** Out-cubic easing. */
export const ease = (t: number) => 1 - (1 - t) ** 3;
