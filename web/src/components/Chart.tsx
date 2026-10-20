// The progression: what was invested, as a hatched bed of stone, and what it
// is worth, as the line above it. The layer between the two is the gain
// (moss) or the loss (garnet). Hand-drawn SVG: a few hundred points, no lib.

import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import { day, dayShort, eur, eurShort, month, monthYear } from "../format";

export interface Pt {
  t: number; // UTC midnight, ms
  v: number; // value
  i: number; // invested
}

const PAD = { top: 12, right: 52, bottom: 28, left: 0 };

/** Round steps for the value axis, from lo to max: 1, 2, 2.5, 5 × 10ⁿ. */
function niceTicks(lo: number, max: number, count: number) {
  if (max <= lo) return [lo];
  const raw = (max - lo) / count;
  const p = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * p).find((s) => s >= raw) ?? 10 * p;
  const out: number[] = [];
  for (let v = Math.floor(lo / step) * step; v <= max + step * 0.01; v += step) out.push(v);
  // The axis always reaches above the highest point.
  if (out[out.length - 1] < max) out.push(out[out.length - 1] + step);
  return out;
}

const DAY = 86_400_000;

/** Date ticks that suit the span: weeks, months or years. */
function timeTicks(t0: number, t1: number, width: number) {
  const span = (t1 - t0) / DAY;
  const out: { t: number; label: string }[] = [];
  const d = new Date(t0);
  if (span <= 45) {
    // Mondays.
    const s = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()));
    while (s.getUTCDay() !== 1) s.setUTCDate(s.getUTCDate() + 1);
    for (; s.getTime() <= t1; s.setUTCDate(s.getUTCDate() + 7)) out.push({ t: s.getTime(), label: dayShort(s.getTime()) });
  } else if (span <= 400) {
    const every = width < 520 ? 3 : width < 760 ? 2 : 1;
    const s = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 1));
    for (; s.getTime() <= t1; s.setUTCMonth(s.getUTCMonth() + 1)) {
      if (s.getUTCMonth() % every) continue;
      const t = s.getTime();
      out.push({ t, label: s.getUTCMonth() === 0 ? monthYear(t) : month(t) });
    }
  } else {
    const years = Math.ceil(span / 365);
    const every = width / years < 56 ? 2 : 1;
    for (let y = d.getUTCFullYear() + 1; Date.UTC(y, 0, 1) <= t1; y++) {
      if (y % every) continue;
      out.push({ t: Date.UTC(y, 0, 1), label: String(y) });
    }
    // Half years when there is room.
    if (years <= 4 && width >= 640) {
      for (let y = d.getUTCFullYear(); Date.UTC(y, 6, 1) <= t1; y++) {
        const t = Date.UTC(y, 6, 1);
        if (t > t0) out.push({ t, label: month(t) });
      }
      out.sort((a, b) => a.t - b.t);
    }
  }
  return out;
}

/** The point nearest to time t (points are sorted). */
function nearest(pts: Pt[], t: number) {
  let lo = 0;
  let hi = pts.length - 1;
  while (hi - lo > 1) {
    const mid = (lo + hi) >> 1;
    if (pts[mid].t < t) lo = mid;
    else hi = mid;
  }
  return t - pts[lo].t < pts[hi].t - t ? lo : hi;
}

export function Chart({ points, animate }: { points: Pt[]; animate: boolean }) {
  const wrap = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [hover, setHover] = useState<number | null>(null);
  const id = useId().replace(/:/g, "");

  useLayoutEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setWidth(Math.round(e.contentRect.width)));
    ro.observe(el);
    setWidth(Math.round(el.getBoundingClientRect().width));
    return () => ro.disconnect();
  }, []);
  useEffect(() => setHover(null), [points]);

  const height = width < 560 ? 230 : 320;
  const right = width < 560 ? 50 : PAD.right;
  const iw = Math.max(0, width - PAD.left - right);
  const ih = height - PAD.top - PAD.bottom;

  const g = useMemo(() => {
    if (points.length < 2 || iw <= 0) return null;
    const t0 = points[0].t;
    const t1 = points[points.length - 1].t;
    const max = Math.max(...points.map((p) => Math.max(p.v, p.i)));
    const vmin = Math.min(...points.map((p) => p.v));
    const vmax = Math.max(...points.map((p) => p.v));
    // The axis starts at 0, unless the value moved little and sits high above
    // it (a month): then it zooms on the value, the invested bed below.
    const lo = vmin > vmax * 0.6 ? Math.max(0, vmin - (vmax - vmin) * 0.25) : 0;
    const top = lo ? vmax : max;
    const yt = niceTicks(lo, top + (top - lo) * 0.04, width < 560 ? 3 : 4);
    const ymin = yt[0];
    const ymax = yt[yt.length - 1] || 1;
    const x = (t: number) => PAD.left + ((t - t0) / (t1 - t0 || 1)) * iw;
    const y = (v: number) => PAD.top + ih - ((Math.max(ymin, v) - ymin) / (ymax - ymin || 1)) * ih;
    const f = (n: number) => n.toFixed(1);
    const line = (k: "v" | "i") => points.map((p, n) => `${n ? "L" : "M"}${f(x(p.t))},${f(y(p[k]))}`).join("");
    const value = line("v");
    const invested = line("i");
    const back = points
      .slice()
      .reverse()
      .map((p) => `L${f(x(p.t))},${f(y(p.i))}`)
      .join("");
    const x0 = f(x(t0));
    const x1 = f(x(t1));
    const base = f(PAD.top + ih);
    return {
      x,
      y,
      yt,
      xt: timeTicks(t0, t1, iw).filter((k) => x(k.t) > 24 && x(k.t) < PAD.left + iw - 24),
      value,
      invested,
      bed: `${invested}L${x1},${base}L${x0},${base}Z`,
      band: `${value}${back}Z`,
      above: `${invested}L${x1},${PAD.top - 4}L${x0},${PAD.top - 4}Z`,
      below: `${invested}L${x1},${base}L${x0},${base}Z`,
    };
  }, [points, iw, ih, width]);

  const last = points[points.length - 1];
  const at = hover ?? null;
  const p = at !== null ? points[at] : null;

  function move(e: React.PointerEvent<SVGSVGElement>) {
    if (!g || points.length < 2) return;
    const r = e.currentTarget.getBoundingClientRect();
    const t0 = points[0].t;
    const t1 = points[points.length - 1].t;
    const t = t0 + ((e.clientX - r.left - PAD.left) / iw) * (t1 - t0);
    setHover(nearest(points, t));
  }

  function key(e: React.KeyboardEvent) {
    const n = points.length - 1;
    const cur = hover ?? n;
    const jump = e.shiftKey ? 30 : 1;
    const to = { ArrowLeft: cur - jump, ArrowRight: cur + jump, Home: 0, End: n }[e.key];
    if (to !== undefined) {
      e.preventDefault();
      setHover(Math.max(0, Math.min(n, to)));
    } else if (e.key === "Escape") setHover(null);
  }

  const label = last
    ? `Progression jusqu'au ${day(last.t)} : valeur ${eur(last.v)}, investi ${eur(last.i)}. Flèches pour parcourir.`
    : "Progression";

  return (
    <div className="chart" ref={wrap} style={{ height }}>
      {g && (
        <svg
          width={width}
          height={height}
          role="img"
          aria-label={label}
          tabIndex={0}
          onPointerMove={move}
          onPointerDown={move}
          onPointerLeave={(e) => e.pointerType === "mouse" && setHover(null)}
          onKeyDown={key}
          onBlur={() => setHover(null)}
        >
          <defs>
            <pattern id={`hatch${id}`} width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
              <line x1="0" y1="0" x2="0" y2="6" className="hatch" />
            </pattern>
            <clipPath id={`above${id}`}>
              <path d={g.above} />
            </clipPath>
            <clipPath id={`below${id}`}>
              <path d={g.below} />
            </clipPath>
          </defs>

          {g.yt.map((v) => (
            <g key={v} className="tick">
              <line x1={PAD.left} x2={PAD.left + iw} y1={g.y(v)} y2={g.y(v)} />
              <text x={PAD.left + iw + 6} y={g.y(v)} dy="0.32em">
                {v === 0 ? "0" : eurShort(v)}
              </text>
            </g>
          ))}
          {g.xt.map((k) => (
            <text key={k.t} className="tick" x={g.x(k.t)} y={height - 8} textAnchor="middle">
              {k.label}
            </text>
          ))}

          <path d={g.bed} className="bed" fill={`url(#hatch${id})`} />
          <path d={g.band} className="gain" clipPath={`url(#above${id})`} />
          <path d={g.band} className="loss" clipPath={`url(#below${id})`} />
          <path d={g.invested} className="invested" />
          <path d={g.value} className={animate ? "value draw" : "value"} pathLength={1} />
          <circle cx={g.x(last.t)} cy={g.y(last.v)} r={3.5} className="end" />

          {p && (
            <g className="cursor">
              <line x1={g.x(p.t)} x2={g.x(p.t)} y1={PAD.top} y2={PAD.top + ih} />
              <circle cx={g.x(p.t)} cy={g.y(p.i)} r={3} className="dot-i" />
              <circle cx={g.x(p.t)} cy={g.y(p.v)} r={4} className="dot-v" />
            </g>
          )}
        </svg>
      )}
      {g && p && (
        <div
          className="tip"
          // Beside the cursor, on the side with room.
          style={{ top: PAD.top, ...(g.x(p.t) > width / 2 ? { right: width - g.x(p.t) + 12 } : { left: g.x(p.t) + 12 }) }}
          aria-live="polite"
        >
          <div className="tip-date">{day(p.t)}</div>
          <div>
            <span className="k">Valeur</span> {eur(p.v)}
          </div>
          <div>
            <span className="k">Investi</span> {eur(p.i)}
          </div>
        </div>
      )}
    </div>
  );
}
