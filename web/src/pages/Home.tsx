// Accueil: where I stand. The total, the progression since the start, where
// it sits, each asset, and this month's money in.

import { useMemo, useRef, useState } from "react";
import { activitiesSince, get, history, useLoad, type Portfolio } from "../api";
import { Allocation, type Part } from "../components/Allocation";
import { Chart, type Pt } from "../components/Chart";
import { Coin } from "../components/Coin";
import { Segmented } from "../components/Segmented";
import { Failed, Loading } from "../components/State";
import { date, eur, num, pct, qty, time, tone } from "../format";

type Period = "1M" | "1A" | "Tout";
const PERIODS: { value: Period; label: string }[] = [
  { value: "1M", label: "1M" },
  { value: "1A", label: "1A" },
  { value: "Tout", label: "Tout" },
];

// The chosen period and view are remembered in this browser, if it lets us.
function remembered<T extends string>(key: string, def: T, ok: readonly T[]): T {
  try {
    const v = localStorage.getItem("cairn." + key) as T | null;
    return v && ok.includes(v) ? v : def;
  } catch {
    return def;
  }
}
function remember(key: string, v: string) {
  try {
    localStorage.setItem("cairn." + key, v);
  } catch {
    /* private window: not kept */
  }
}

const utc = (d: string) => Date.parse(d + "T00:00:00Z");

/** The points of a period; "Tout" keeps one a week when there are many. */
function slice(all: Pt[], period: Period): Pt[] {
  if (!all.length) return all;
  const last = new Date(all[all.length - 1].t);
  const y = last.getUTCFullYear();
  const m = last.getUTCMonth();
  const d = last.getUTCDate();
  if (period === "1M") return all.filter((p) => p.t >= Date.UTC(y, m - 1, d));
  if (period === "1A") return all.filter((p) => p.t >= Date.UTC(y - 1, m, d));
  if (all.length <= 900) return all;
  return all.filter((p, n) => n === 0 || n === all.length - 1 || new Date(p.t).getUTCDay() === 0);
}

/** A big amount, the unit set apart. */
function Big({ text }: { text: string }) {
  const m = text.match(/^(.*?)\s*€$/);
  return m ? (
    <>
      {m[1]}
      <span className="unit">€</span>
    </>
  ) : (
    <>{text}</>
  );
}

export function Home() {
  const pf = useLoad("portfolio", (s) => get<Portfolio>("portfolio", s));
  const hist = useLoad("history", history);
  const monthStart = useMemo(() => {
    const n = new Date();
    return new Date(Date.UTC(n.getUTCFullYear(), n.getUTCMonth(), 1));
  }, []);
  const acts = useLoad("month", (s) => activitiesSince(monthStart, s));

  const [period, setPeriod] = useState<Period>(() => remembered("period", "Tout", ["1M", "1A", "Tout"] as const));
  const [view, setView] = useState<"assets" | "sources">(() => remembered("view", "assets", ["assets", "sources"] as const));
  // The value line draws itself once, on the first chart.
  const drawn = useRef(false);

  const all = useMemo<Pt[]>(
    () => (hist.data?.points ?? []).map((p) => ({ t: utc(p.date), v: num(p.value), i: num(p.invested) })),
    [hist.data],
  );
  const points = useMemo(() => slice(all, period), [all, period]);

  const month = useMemo(() => {
    if (!all.length) return null;
    const before = all.filter((p) => p.t < monthStart.getTime());
    const base = before.length ? before[before.length - 1].i : 0;
    const rewards = (acts.data ?? [])
      .filter((a) => (a.type === "reward" || a.type === "interest") && a.base?.asset.kind === "crypto")
      .reduce((s, a) => s + num(a.cost), 0);
    return { invested: all[all.length - 1].i - base, rewards };
  }, [all, acts.data, monthStart]);

  const error = pf.error ?? hist.error;
  if (error && (!pf.data || !hist.data))
    return (
      <Failed
        error={error}
        retry={() => {
          pf.reload();
          hist.reload();
        }}
      />
    );
  if (!pf.data || !hist.data) return <Loading />;

  const p = pf.data;
  const value = num(p.total.value);
  const invested = num(p.total.invested);
  const gain = value - invested;

  const first = points[0];
  const last = points[points.length - 1];
  const periodGain = first && last ? last.v - last.i - (first.v - first.i) : 0;

  const parts: Part[] =
    view === "assets"
      ? p.assets.map((a) => ({ key: a.asset.symbol, label: a.asset.symbol, value: num(a.value) }))
      : p.sources.map((s) => ({ key: s.id, label: s.name, value: num(s.value) }));

  const animate = !drawn.current;
  drawn.current = true;

  return (
    <div className="home">
      <section className="hero" aria-labelledby="h-home">
        <h1 id="h-home" className="sr">
          Où j'en suis
        </h1>
        <p className="total num" aria-label={`Valeur : ${eur(value)}`}>
          <Big text={eur(value)} />
        </p>
        <p className="gain num">
          <span className={tone(gain)}>{eur(gain, { sign: true })}</span>
          {invested > 0 && <span className={tone(gain)}>{pct(gain / invested, { sign: true })}</span>}
          <span className="muted">sur {eur(invested)} investis</span>
        </p>
        <dl className="parts num">
          <div>
            <dt>Latente</dt>
            <dd>{eur(p.total.unrealized, { sign: true })}</dd>
          </div>
          <div>
            <dt>Réalisée</dt>
            <dd>{eur(p.total.realized, { sign: true })}</dd>
          </div>
          <div>
            <dt>Récompenses</dt>
            <dd>{eur(p.total.rewards)}</dd>
          </div>
        </dl>
      </section>

      <section className="progress" aria-label="Progression">
        <div className="progress-head">
          <div className="legend" aria-hidden>
            <span className="lg-v">Valeur</span>
            <span className="lg-i">Investi</span>
          </div>
          <span className={`period-gain num ${tone(periodGain)}`} title="Gain sur la période">
            {period !== "Tout" && points.length > 1 && eur(periodGain, { sign: true })}
          </span>
          <Segmented
            label="Période"
            options={PERIODS}
            value={period}
            onChange={(v) => {
              setPeriod(v);
              remember("period", v);
            }}
          />
        </div>
        {points.length > 1 ? <Chart points={points} animate={animate} /> : <p className="empty">Pas encore d'historique.</p>}
      </section>

      <div className="split">
        <section className="where" aria-labelledby="h-alloc">
          <div className="head">
            <h2 id="h-alloc">Répartition</h2>
            <Segmented
              label="Répartition par"
              options={[
                { value: "assets", label: "Actifs" },
                { value: "sources", label: "Plateformes" },
              ]}
              value={view}
              onChange={(v) => {
                setView(v);
                remember("view", v);
              }}
            />
          </div>
          <Allocation parts={parts} />
        </section>

        <section className="month" aria-labelledby="h-month">
          <div className="head">
            <h2 id="h-month">Ce mois-ci</h2>
          </div>
          <dl className="num">
            <div>
              <dt>Apports</dt>
              <dd>{month ? eur(month.invested, { sign: true }) : "–"}</dd>
            </div>
            <div>
              <dt>Récompenses</dt>
              <dd>{acts.error ? <span title={acts.error}>–</span> : month && acts.data ? eur(month.rewards) : "…"}</dd>
            </div>
          </dl>
        </section>
      </div>

      <section className="assets" aria-labelledby="h-assets">
        <div className="head">
          <h2 id="h-assets">Actifs</h2>
        </div>
        <table className="table num">
          <thead>
            <tr>
              <th scope="col">Actif</th>
              <th scope="col" className="wide">
                Quantité
              </th>
              <th scope="col">Valeur</th>
              <th scope="col" className="wide">
                Coût
              </th>
              <th scope="col">Plus-value</th>
              <th scope="col" className="mid">
                Part
              </th>
            </tr>
          </thead>
          <tbody>
            {p.assets.map((a) => {
              const u = num(a.unrealized);
              const c = num(a.cost);
              return (
                <tr key={a.asset.symbol}>
                  <th scope="row">
                    <span className="asset">
                      <Coin symbol={a.asset.symbol} />
                      <span>
                        <span className="sym">{a.asset.symbol}</span>
                        <span className="sub">{qty(a.qty)}</span>
                      </span>
                    </span>
                  </th>
                  <td className="wide">{qty(a.qty)}</td>
                  <td>
                    {eur(a.value)}
                    <span className="sub mid-hide">{pct(num(a.share))}</span>
                  </td>
                  <td className="wide muted">{eur(a.cost)}</td>
                  <td className={tone(u)}>
                    {eur(u, { sign: true })}
                    {c > 0 && <span className="sub">{pct(u / c, { sign: true })}</span>}
                  </td>
                  <td className="mid">
                    <span className="share-cell">
                      <span className="share-bar" style={{ width: `${Math.max(2, num(a.share) * 100)}%` }} />
                      {pct(num(a.share))}
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </section>

      <footer className="foot">
        <span>
          État au {date(p.at)}, {time(p.at)}
        </span>
        {p.unpriced.length > 0 && <span>Sans prix : {p.unpriced.map((a) => a.symbol).join(", ")}</span>}
      </footer>
    </div>
  );
}
