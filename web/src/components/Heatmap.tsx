// Monthly returns, a year a row: moss for a month that grew, garnet for one
// that fell, deeper the further it went. The year's own return at the end.

import { useState } from "react";
import { monthLong, pct } from "../format";
import { utc } from "../lib/chart";

const MONTHS = ["J", "F", "M", "A", "M", "J", "J", "A", "S", "O", "N", "D"];

export function Heatmap({ months }: { months: { m: string; ret: number }[] }) {
  const [picked, setPicked] = useState<string | null>(null);
  const by = new Map(months.map((m) => [m.m, m.ret]));
  const years = [...new Set(months.map((m) => Number(m.m.slice(0, 4))))].sort((a, b) => b - a);
  const cell = (r: number | undefined) => {
    if (r === undefined) return undefined;
    const k = Math.min(1, Math.abs(r) / 0.25);
    return { "--k": `${Math.round(12 + k * 78)}%` } as React.CSSProperties;
  };
  const short = (r: number) => {
    const p = Math.round(r * 100);
    return p === 0 ? "0" : (p > 0 ? "+" : "−") + Math.abs(p);
  };
  const pick = picked !== null ? by.get(picked) : undefined;
  return (
    <div className="heat">
      <table>
        <thead>
          <tr>
            <th scope="col">
              <span className="sr">Année</span>
            </th>
            {MONTHS.map((m, i) => (
              <th key={i} scope="col" aria-label={monthLong(Date.UTC(2000, i, 1)).replace(" 2000", "")}>
                {m}
              </th>
            ))}
            <th scope="col" className="heat-year">
              Année
            </th>
          </tr>
        </thead>
        <tbody>
          {years.map((y) => {
            let year = 1;
            let any = false;
            return (
              <tr key={y}>
                <th scope="row">{y}</th>
                {MONTHS.map((_, i) => {
                  const key = `${y}-${String(i + 1).padStart(2, "0")}`;
                  const r = by.get(key);
                  if (r !== undefined) (year *= 1 + r), (any = true);
                  return (
                    <td key={i}>
                      {r === undefined ? (
                        <span className="heat-cell none" />
                      ) : (
                        <span
                          role="img"
                          className={`heat-cell ${r >= 0 ? "up" : "down"} ${Math.abs(r) > 0.14 ? "deep" : ""} ${picked === key ? "on" : ""}`}
                          style={cell(r)}
                          aria-label={`${monthLong(utc(key))} : ${pct(r, { sign: true })}`}
                          onClick={() => setPicked(picked === key ? null : key)}
                          onMouseEnter={() => setPicked(key)}
                          onMouseLeave={() => setPicked(null)}
                        >
                          <span aria-hidden>{short(r)}</span>
                        </span>
                      )}
                    </td>
                  );
                })}
                <td className={`heat-year num ${any ? (year >= 1 ? "up" : "down") : ""}`}>{any ? pct(year - 1, { sign: true }) : ""}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
      <p className="chart-note num" aria-live="polite">
        {picked && pick !== undefined ? (
          <>
            {monthLong(utc(picked))} <b className={pick >= 0 ? "up" : "down"}>{pct(pick, { sign: true })}</b>
          </>
        ) : (
          "Chaque mois, hors apports et retraits"
        )}
      </p>
    </div>
  );
}
