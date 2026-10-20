// One line over time, drawn by hand in SVG: a thin smooth stroke with a
// faint wash under it, a dashed companion (what was invested, an average
// cost), optional buy and sale marks. Pointer, touch or arrow keys scrub
// it; the parent shows what is under the cursor. When the data changes,
// the line flows from its old shape to the new one.

import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ease, nearest, niceTicks, reducedMotion, sampleAt, smooth, timeTicks } from "../lib/chart";

export interface Mark {
  t: number;
  y: number;
  buy: boolean;
}

export interface LineChartProps {
  t: number[];
  y: number[];
  /** A dashed line on the same days. */
  y2?: number[];
  /** gain, loss or neutral: the line's color. */
  tone: "up" | "down" | "ink";
  /** A dashed level across the chart (0 for returns). */
  level?: number;
  /** Starts the axis at 0 when the values allow it. */
  fromZero?: boolean;
  marks?: Mark[];
  axis: (v: number) => string;
  height: number;
  /** Changes when the data means something else (a period, a view): the line flows. */
  dataKey: string;
  label: string;
  scrub: number | null;
  onScrub: (i: number | null) => void;
  tip?: (i: number) => React.ReactNode;
}

const PAD = { top: 16, bottom: 26 };

/** A smooth path through the finite stretches of ys (gaps where NaN). */
function pieces(xs: Float64Array, ys: Float64Array): string {
  let out = "";
  let start = -1;
  for (let i = 0; i <= ys.length; i++) {
    const ok = i < ys.length && Number.isFinite(ys[i]);
    if (ok && start < 0) start = i;
    if (!ok && start >= 0) {
      out += smooth(xs.subarray(start, i), ys.subarray(start, i));
      start = -1;
    }
  }
  return out;
}
const MORPH = 520;

export function LineChart(p: LineChartProps) {
  const wrap = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const id = useId().replace(/:/g, "");

  useLayoutEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setWidth(Math.round(e.contentRect.width)));
    ro.observe(el);
    setWidth(Math.round(el.getBoundingClientRect().width));
    return () => ro.disconnect();
  }, []);

  const narrow = width < 560;
  const axisW = narrow ? 6 : 56;
  const iw = Math.max(0, width - axisW);
  // Tall charts are a little shorter on a phone.
  const height = narrow && p.height > 200 ? Math.round(p.height * 0.8) : p.height;
  const ih = height - PAD.top - PAD.bottom;

  // Target geometry.
  const g = useMemo(() => {
    const n = p.t.length;
    if (n < 2 || iw <= 0) return null;
    let lo = Infinity;
    let hi = -Infinity;
    for (const arr of [p.y, p.y2 ?? []]) for (const v of arr) if (Number.isFinite(v)) (lo = Math.min(lo, v)), (hi = Math.max(hi, v));
    if (p.level !== undefined) (lo = Math.min(lo, p.level)), (hi = Math.max(hi, p.level));
    if (p.fromZero && lo >= 0 && lo < hi * 0.55) lo = 0;
    const pad = (hi - lo) * 0.06 || Math.abs(hi) * 0.05 || 1;
    const ticks = niceTicks(lo === 0 ? 0 : lo - pad, hi + pad, narrow ? 3 : 4);
    const y0 = lo === 0 ? 0 : Math.max(ticks[0], lo - pad);
    const y1 = Math.min(ticks[ticks.length - 1], hi + pad * 2);
    const t0 = p.t[0];
    const t1 = p.t[n - 1];
    const x = (t: number) => ((t - t0) / (t1 - t0 || 1)) * iw;
    const y = (v: number) => PAD.top + ih - ((v - y0) / (y1 - y0 || 1)) * ih;
    const xs = Float64Array.from(p.t, x);
    const ys = Float64Array.from(p.y, y);
    const ys2 = p.y2 ? Float64Array.from(p.y2, y) : null;
    return {
      x,
      y,
      xs,
      ys,
      ys2,
      ticks: ticks.filter((v) => v >= y0 && v <= y1),
      xt: timeTicks(t0, t1, iw).filter((k) => x(k.t) > 28 && x(k.t) < iw - 28),
    };
  }, [p.t, p.y, p.y2, p.level, p.fromZero, iw, ih, narrow]);

  // The flow from the shape on screen to the new one.
  const drawn = useRef<{ xs: Float64Array; ys: Float64Array; ys2: Float64Array | null } | null>(null);
  const from = useRef<{ ys: Float64Array; ys2: Float64Array | null } | null>(null);
  const [k, setK] = useState(1);
  const lastKey = useRef(p.dataKey);
  useLayoutEffect(() => {
    if (!g) return;
    const d = drawn.current;
    if (!d || lastKey.current === p.dataKey || reducedMotion()) {
      lastKey.current = p.dataKey;
      from.current = null;
      setK(1);
      return;
    }
    lastKey.current = p.dataKey;
    from.current = {
      ys: g.xs.map((x) => sampleAt(d.xs, d.ys, x)),
      ys2: g.ys2 ? g.xs.map((x) => sampleAt(d.xs, d.ys2 ?? d.ys, x)) : null,
    };
    let raf = 0;
    const start = performance.now();
    const step = (now: number) => {
      const t = Math.min(1, (now - start) / MORPH);
      setK(t);
      if (t < 1) raf = requestAnimationFrame(step);
    };
    setK(0);
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [g, p.dataKey]);

  const shape = useMemo(() => {
    if (!g) return null;
    const f = from.current;
    const e = ease(k);
    const mix = (a: Float64Array | null, b: Float64Array | null) => (a && b && k < 1 ? b.map((v, i) => a[i] + (v - a[i]) * e) : b);
    const ys = mix(f?.ys ?? null, g.ys)!;
    const ys2 = mix(f?.ys2 ?? null, g.ys2);
    return { ys, ys2, line: smooth(g.xs, ys), line2: ys2 ? pieces(g.xs, ys2) : "" };
  }, [g, k]);

  useEffect(() => {
    if (g && shape) drawn.current = { xs: g.xs, ys: shape.ys, ys2: shape.ys2 };
  }, [g, shape]);

  const n = p.t.length;
  const at = p.scrub !== null && p.scrub < n ? p.scrub : null;

  function move(e: React.PointerEvent<SVGSVGElement>) {
    if (!g) return;
    const r = e.currentTarget.getBoundingClientRect();
    p.onScrub(nearest(g.xs, e.clientX - r.left));
  }
  function key(e: React.KeyboardEvent) {
    const cur = at ?? n - 1;
    const jump = e.shiftKey ? 30 : 1;
    const to = { ArrowLeft: cur - jump, ArrowRight: cur + jump, Home: 0, End: n - 1 }[e.key];
    if (to !== undefined) {
      e.preventDefault();
      p.onScrub(Math.max(0, Math.min(n - 1, to)));
    } else if (e.key === "Escape") p.onScrub(null);
  }

  const bottom = PAD.top + ih;
  const cx = at !== null && g ? g.xs[at] : 0;

  return (
    <div className={`lc tone-${p.tone}`} ref={wrap} style={{ height }}>
      {g && shape && (
        <svg
          width={width}
          height={height}
          role="img"
          aria-label={p.label}
          tabIndex={0}
          onPointerMove={move}
          onPointerDown={move}
          onPointerLeave={(e) => e.pointerType === "mouse" && p.onScrub(null)}
          onPointerCancel={() => p.onScrub(null)}
          onKeyDown={key}
          onBlur={() => p.onScrub(null)}
        >
          <defs>
            <linearGradient id={`wash${id}`} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0" className="wash-top" />
              <stop offset="1" className="wash-bottom" />
            </linearGradient>
          </defs>
          {g.ticks.map((v) => (
            <g key={v} className="lc-tick">
              <line x1={0} x2={iw} y1={g.y(v)} y2={g.y(v)} />
              {!narrow && (
                <text x={iw + 10} y={g.y(v)} dy="0.32em">
                  {p.axis(v)}
                </text>
              )}
            </g>
          ))}
          {g.xt.map((t) => (
            <text key={t.t} className="lc-date" x={g.x(t.t)} y={height - 6} textAnchor="middle">
              {t.label}
            </text>
          ))}
          {p.level !== undefined && <line className="lc-level" x1={0} x2={iw} y1={g.y(p.level)} y2={g.y(p.level)} />}
          <path className="lc-wash" d={`${shape.line}L${g.xs[n - 1]},${bottom}L0,${bottom}Z`} fill={`url(#wash${id})`} />
          {shape.line2 && <path className="lc-line2" d={shape.line2} />}
          <path className="lc-line" d={shape.line} />
          {p.marks && k === 1 && (
            <g className="lc-marks">
              {p.marks.map((m, i) => (
                <circle key={i} cx={g.x(m.t)} cy={g.y(m.y)} r={narrow ? 3 : 3.5} className={m.buy ? "buy" : "sell"} />
              ))}
            </g>
          )}
          {at === null ? (
            k === 1 && <circle className="lc-end" cx={g.xs[n - 1]} cy={shape.ys[n - 1]} r={4} />
          ) : (
            <g className="lc-cursor">
              <line x1={cx} x2={cx} y1={PAD.top - 8} y2={bottom} />
              {shape.ys2 && Number.isFinite(shape.ys2[at]) && <circle className="lc-dot2" cx={cx} cy={shape.ys2[at]} r={3.5} />}
              <circle className="lc-dot" cx={cx} cy={shape.ys[at]} r={5} />
            </g>
          )}
        </svg>
      )}
      {g && at !== null && p.tip && (
        <div
          className="lc-tip"
          style={cx > width / 2 ? { right: width - cx + 14 } : { left: cx + 14 }}
          aria-live="polite"
        >
          {p.tip(at)}
        </div>
      )}
    </div>
  );
}
