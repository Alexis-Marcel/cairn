// Money put in each month, as bars over a line at zero; taken out below it.
// The dashed line is the monthly average over the bars shown.

import { useLayoutEffect, useRef, useState } from "react";
import { eur, eurShort, month, monthLong } from "../format";
import { utc } from "../lib/chart";

export function MonthBars({ months }: { months: { m: string; in: number; out: number }[] }) {
  const wrap = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [at, setAt] = useState<number | null>(null);
  useLayoutEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setWidth(Math.round(e.contentRect.width)));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const shown = months.slice(-(width < 480 ? 12 : 24));
  const height = 168;
  const top = 8;
  const base = 20;
  const hi = Math.max(1, ...shown.map((m) => m.in));
  const anyOut = shown.some((m) => m.out > 0);
  const ih = height - top - base;
  // Money put in sets the scale; withdrawals hang below, cut when longer
  // than the room left (a big sale would flatten every month's bar).
  const zero = top + ih * (anyOut ? 0.76 : 1);
  const scale = (zero - top) / hi;
  const room = top + ih - zero;
  const slot = width / Math.max(1, shown.length);
  const bw = Math.max(3, Math.min(14, slot * 0.5));
  const avg = shown.reduce((s, m) => s + m.in - m.out, 0) / Math.max(1, shown.length);
  const m = at !== null ? shown[at] : null;
  return (
    <div className="bars" ref={wrap}>
      {width > 0 && (
        <svg width={width} height={height} role="img" aria-label={`Apports des ${shown.length} derniers mois, ${eur(avg)} par mois en moyenne`} onPointerLeave={() => setAt(null)}>
          {shown.map((x, i) => {
            const cx = slot * i + slot / 2;
            return (
              <g key={x.m} className={at === i ? "bar on" : "bar"} onPointerEnter={() => setAt(i)} onPointerDown={() => setAt(i)}>
                <rect className="hit" x={slot * i} y={0} width={slot} height={height} />
                {x.in > 0 && <rect className="in" x={cx - bw / 2} y={zero - x.in * scale} width={bw} height={Math.max(1, x.in * scale)} rx={Math.min(4, bw / 2)} />}
                {x.out > 0 && (
                  <>
                    <rect className="out" x={cx - bw / 2} y={zero} width={bw} height={Math.max(1, Math.min(room, x.out * scale))} rx={Math.min(4, bw / 2)} />
                    {x.out * scale > room && <line className="cut" x1={cx - bw / 2 - 1} x2={cx + bw / 2 + 1} y1={zero + room - 5} y2={zero + room - 8} />}
                  </>
                )}
                {(i === 0 || x.m.endsWith("-01") || i === shown.length - 1) && (
                  <text x={cx} y={height - 4} textAnchor={i === 0 ? "start" : i === shown.length - 1 ? "end" : "middle"} dx={i === 0 ? -bw / 2 : i === shown.length - 1 ? bw / 2 : 0}>
                    {x.m.endsWith("-01") ? x.m.slice(0, 4) : month(utc(x.m))}
                  </text>
                )}
              </g>
            );
          })}
          <line className="zero" x1={0} x2={width} y1={zero} y2={zero} />
          {avg > 0 && <line className="avg" x1={0} x2={width} y1={zero - avg * scale} y2={zero - avg * scale} />}
        </svg>
      )}
      <p className="chart-note num" aria-live="polite">
        {m ? (
          <>
            {monthLong(utc(m.m))} <b>{eur(m.in - m.out, { sign: true })}</b>
            {m.out > 0 && m.in > 0 && <span className="muted"> ({eur(m.in)} apportés, {eur(m.out)} retirés)</span>}
          </>
        ) : (
          <>
            <b>{eur(avg)}</b> par mois en moyenne <span className="muted">· {eurShort(shown.reduce((s, x) => s + x.in - x.out, 0))} sur {shown.length} mois</span>
          </>
        )}
      </p>
    </div>
  );
}
