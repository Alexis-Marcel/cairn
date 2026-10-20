// Where the value comes from, month after month: the money put in at the
// bottom (with Fundarr's corrections), then the income received, then what
// the market added (moss) or took (garnet). The top edge is the value.

import { useId, useLayoutEffect, useRef, useState } from "react";
import { eur, eurShort, monthLong } from "../format";
import { nearest, niceTicks, smooth, utc } from "../lib/chart";

export interface SplitPoint {
  m: string;
  value: number;
  invested: number;
  adjusted: number;
  earned: number;
}

export function SplitArea({ points }: { points: SplitPoint[] }) {
  const wrap = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [at, setAt] = useState<number | null>(null);
  const id = useId().replace(/:/g, "");
  useLayoutEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setWidth(Math.round(e.contentRect.width)));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const height = 168;
  const top = 8;
  const bottom = height - 20;
  const n = points.length;
  const hi = Math.max(1, ...points.map((p) => Math.max(p.value, p.invested + p.earned)));
  const ticks = niceTicks(0, hi, 3);
  const ymax = ticks[ticks.length - 1];
  const xs = points.map((_, i) => (n > 1 ? (i / (n - 1)) * width : width / 2));
  const y = (v: number) => bottom - (Math.max(0, v) / ymax) * (bottom - top);
  const inv = points.map((p) => y(p.invested));
  const earn = points.map((p) => y(p.invested + p.earned));
  const val = points.map((p) => y(p.value));
  const band = (upper: number[], lower: number[]) => {
    const up = smooth(xs, upper);
    const rx = [...xs].reverse();
    const lw = smooth(rx, [...lower].reverse()).replace(/^M/, "L");
    return `${up}${lw}Z`;
  };
  const floor = points.map(() => bottom);
  const p = at !== null ? points[at] : points[n - 1];
  const market = p ? p.value - p.invested - p.earned : 0;
  return (
    <div className="split-area" ref={wrap}>
      {width > 0 && n > 1 && (
        <svg
          width={width}
          height={height}
          role="img"
          aria-label="Valeur décomposée en apports, revenus et marché"
          onPointerMove={(e) => setAt(nearest(xs, e.clientX - e.currentTarget.getBoundingClientRect().left))}
          onPointerDown={(e) => setAt(nearest(xs, e.clientX - e.currentTarget.getBoundingClientRect().left))}
          onPointerLeave={() => setAt(null)}
        >
          <defs>
            <clipPath id={`above${id}`}>
              <path d={`${smooth(xs, earn)}L${width},0L0,0Z`} />
            </clipPath>
            <clipPath id={`below${id}`}>
              <path d={`${smooth(xs, earn)}L${width},${height}L0,${height}Z`} />
            </clipPath>
          </defs>
          {ticks.slice(1).map((v) => (
            <g key={v} className="lc-tick">
              <line x1={0} x2={width} y1={y(v)} y2={y(v)} />
              <text x={0} y={y(v) - 5}>
                {eurShort(v)}
              </text>
            </g>
          ))}
          <path className="band-invested" d={band(inv, floor)} />
          <path className="band-earned" d={band(earn, inv)} />
          <path className="band-market up" d={band(val, earn)} clipPath={`url(#above${id})`} />
          <path className="band-market down" d={band(earn, val)} clipPath={`url(#below${id})`} />
          <path className="band-top" d={smooth(xs, val)} />
          {at !== null && <line className="band-cursor" x1={xs[at]} x2={xs[at]} y1={top} y2={bottom} />}
          <text className="lc-date" x={0} y={height - 4}>
            {points[0].m.slice(0, 4)}
          </text>
          <text className="lc-date" x={width} y={height - 4} textAnchor="end">
            {points[n - 1].m.slice(0, 4)}
          </text>
        </svg>
      )}
      {p && (
        <dl className="split-key num" aria-live="polite">
          <div>
            <dt>
              <i className="sw invested" />
              Apports
            </dt>
            <dd>{eur(p.invested - p.adjusted)}</dd>
          </div>
          {Math.abs(p.adjusted) >= 0.5 && (
            <div title="Corrections de Fundarr : ni apport ni gain">
              <dt>
                <i className="sw invested" />
                Ajustements
              </dt>
              <dd>{eur(p.adjusted, { sign: true })}</dd>
            </div>
          )}
          <div>
            <dt>
              <i className="sw earned" />
              Revenus
            </dt>
            <dd>{eur(p.earned)}</dd>
          </div>
          <div>
            <dt>
              <i className={`sw market ${market < 0 ? "down" : "up"}`} />
              Marché
            </dt>
            <dd className={market < 0 ? "down" : "up"}>{eur(market, { sign: true })}</dd>
          </div>
          <div className="split-when">{at !== null ? monthLong(utc(p.m)) : "Aujourd'hui"}</div>
        </dl>
      )}
    </div>
  );
}
