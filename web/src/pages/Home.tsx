// Accueil: the value and how it moved over a period (the big figure is the
// chart's readout), the key figures, the coins and where the value sits,
// then the statistics.

import { useMemo, useState } from "react";
import { get, useLoad, type CashStat, type CoinStat, type Overview, type PeriodKey, type Portfolio, type Scope, type Stats, type TaxFR } from "../api";
import { Adjusted } from "../components/Adjusted";
import { Allocation } from "../components/Allocation";
import { Coin } from "../components/Coin";
import { Heatmap } from "../components/Heatmap";
import { LineChart } from "../components/LineChart";
import { MonthBars } from "../components/MonthBars";
import { Num, useTween } from "../components/Num";
import { Ranking } from "../components/Ranking";
import { ScopeSwitch } from "../components/ScopeSwitch";
import { Segmented } from "../components/Segmented";
import { Spark } from "../components/Spark";
import { SplitArea } from "../components/SplitArea";
import { Failed, Skel, SkelRows, Slow } from "../components/State";
import { CLASS, label } from "../assets";
import { date, day, eur, eurShort, monthLong, pct, qty, time, tone } from "../format";
import { utc } from "../lib/chart";
import { remember, remembered } from "../lib/remember";

export const PERIODS: { value: PeriodKey; label: string }[] = [
  { value: "7J", label: "7J" },
  { value: "1M", label: "1M" },
  { value: "YTD", label: "YTD" },
  { value: "1A", label: "1A" },
  { value: "Tout", label: "Tout" },
];
export const SINCE: Record<PeriodKey, string> = {
  "7J": "en 7 jours",
  "1M": "en 1 mois",
  YTD: "depuis janvier",
  "1A": "en 1 an",
  Tout: "depuis le début",
};

type View = "value" | "perf";

/**
 * The gain over what was at stake: the opening value plus the money put in
 * since, net of what was taken out (never below the opening value). As the
 * server's period figure (internal/perf.Simple).
 */
function simple(open: { v: number; i: number }, p: { v: number; i: number }): number | null {
  const base = open.v + Math.max(0, p.i - open.i);
  return base > 0.5 ? (p.v - p.i - (open.v - open.i)) / base : null;
}

/** Up or down, as a small triangle. */
export function Arrow({ n }: { n: number }) {
  return (
    <svg className={`arrow ${n < 0 ? "down" : "up"}`} viewBox="0 0 12 12" aria-hidden>
      <path d={n < 0 ? "M6 9.5 2 4.5h8z" : "M6 2.5 10 7.5H2z"} />
    </svg>
  );
}

/** A big amount, rolling to its value; the euro sign set apart. */
export function BigEuros({ value, instant }: { value: number; instant?: boolean }) {
  const v = useTween(value, instant);
  const text = eur(v, { cents: true });
  const m = text.match(/^(.*?)(,\d\d)?\s*€$/);
  if (!m) return <>{text}</>;
  return (
    <>
      {m[1]}
      {m[2] && <span className="cents">{m[2]}</span>}
      <span className="unit">€</span>
    </>
  );
}

/** The scope figures cover, and how to change it when there is a choice. */
export interface Scoped {
  scope: Scope;
  onScope?: (s: Scope) => void;
}

export function Home({ rev, go, scope, onScope }: { rev: number; go: (path: string) => void } & Scoped) {
  const q = "?scope=" + scope;
  const ov = useLoad("overview" + q + rev, (s) => get<Overview>("overview" + q, s));
  const pf = useLoad("portfolio" + q + rev, (s) => get<Portfolio>("portfolio" + q, s));
  const st = useLoad("stats" + q + rev, (s) => get<Stats>("stats" + q, s));
  const lastYear = new Date().getFullYear() - 1;
  const tax = useLoad(ov.data ? "tax" + rev : null, (s) => get<TaxFR>(`tax/fr?year=${lastYear}`, s));

  const [period, setPeriod] = useState<PeriodKey>(() => remembered("period", "1A", ["7J", "1M", "YTD", "1A", "Tout"] as const));
  const [view, setView] = useState<View>(() => remembered("view", "value", ["value", "perf"] as const));
  const [by, setBy] = useState<"assets" | "classes" | "sources">(() => remembered("by", "assets", ["assets", "classes", "sources"] as const));
  const [scrub, setScrub] = useState<number | null>(null);

  const o = ov.data;
  const per = o?.periods.find((p) => p.key === period);

  // The period's points, its opening first (nothing before "Tout").
  const series = useMemo(() => {
    if (!o || !per) return null;
    const all = o.points;
    const raw = per.key === "Tout" ? all : all.slice(per.from);
    // Long spans keep about 400 points: a clean line, exact values.
    const every = Math.ceil(raw.length / 400);
    const pts = every > 1 ? raw.filter((_, n) => n % every === 0 || n === raw.length - 1) : raw;
    const open = per.key === "Tout" ? { d: "", v: 0, i: 0, x: 1 } : all[per.from];
    return {
      pts,
      open,
      t: pts.map((p) => utc(p.d)),
      value: pts.map((p) => p.v),
      invested: pts.map((p) => p.i),
      perf: pts.map((p) => (p.x / (open.x || 1) - 1) * 100),
    };
  }, [o, per]);

  // A new scope: the cursor and its figures start over.
  const [lastScope, setLastScope] = useState(scope);
  if (lastScope !== scope) {
    setLastScope(scope);
    setScrub(null);
  }

  if (ov.error && !o) return <Failed error={ov.error} retry={ov.reload} />;
  if (!o || !per || !series) return <HomeSkeleton />;

  const { pts, open } = series;
  const at = scrub !== null && scrub < pts.length ? pts[scrub] : null;
  const gain = at ? at.v - at.i - (open.v - open.i) : per.gain;
  // Valeur: the gain over what was at stake; Performance: time-weighted.
  const twr = at ? at.x / (open.x || 1) - 1 : per.twr;
  const pctOf = at ? simple(open, at) : per.pct;

  const adjusted = Math.abs(o.total.adjusted) >= 0.5;

  const changePeriod = (v: PeriodKey) => {
    setPeriod(v);
    setScrub(null);
    remember("period", v);
  };

  return (
    <div className="home">
      <section className="hero" aria-labelledby="h-value">
        {onScope ? (
          <div className="hero-when hero-scope">
            <h1 id="h-value" className="sr">
              Portefeuille
            </h1>
            <ScopeSwitch scope={scope} onChange={onScope} />
            {at && <span className="hero-date">{day(utc(at.d))}</span>}
          </div>
        ) : (
          <h1 id="h-value" className="hero-when">
            {at ? day(utc(at.d)) : "Portefeuille"}
          </h1>
        )}
        <p className="hero-value num">
          <BigEuros value={at ? at.v : o.total.value} instant={!!at} />
        </p>
        {view === "value" ? (
          <p className={`hero-change num ${tone(gain)}`}>
            <Arrow n={gain} />
            <Num value={gain} instant={!!at} format={(v) => eur(v, { sign: true, cents: true })} />
            {pctOf !== null && (
              <span>
                (<Num value={pctOf} instant={!!at} format={(v) => pct(v, { sign: true })} />)
              </span>
            )}
            <span className="since">{at && open.d ? "depuis le " + day(utc(open.d)) : SINCE[period]}</span>
          </p>
        ) : (
          <p className={`hero-change num ${tone(twr)}`}>
            <Arrow n={twr} />
            <Num value={twr} instant={!!at} format={(v) => pct(v, { sign: true })} />
            <span className="since">
              performance {at && open.d ? "depuis le " + day(utc(open.d)) : SINCE[period]}, hors apports
            </span>
          </p>
        )}
      </section>

      <section className="graph" aria-label="Évolution">
        <LineChart
          t={series.t}
          y={view === "value" ? series.value : series.perf}
          y2={view === "value" ? series.invested : undefined}
          level={view === "perf" ? 0 : undefined}
          fromZero={view === "value"}
          tone={per.gain >= 0 ? "up" : "down"}
          axis={view === "value" ? (v) => (v === 0 ? "0" : eurShort(v)) : (v) => `${v > 0 ? "+" : v < 0 ? "−" : ""}${Math.abs(Math.round(v))} %`}
          height={320}
          dataKey={period + view + scope}
          label={`${view === "value" ? "Valeur et apports" : "Performance"} ${SINCE[period]}. Flèches pour parcourir.`}
          scrub={scrub}
          onScrub={setScrub}
          tip={(i) => {
            const p = pts[i];
            const g = p.v - p.i;
            return (
              <dl>
                <div>
                  <dt>Valeur</dt>
                  <dd>{eur(p.v)}</dd>
                </div>
                <div>
                  <dt>
                    <i className="dash" aria-hidden />
                    Investi
                  </dt>
                  <dd>{eur(p.i)}</dd>
                </div>
                <div>
                  <dt>Gain total</dt>
                  <dd className={tone(g)}>{eur(g, { sign: true })}</dd>
                </div>
              </dl>
            );
          }}
        />
        <div className="graph-bar">
          <Segmented label="Période" className="chips" options={PERIODS} value={period} onChange={changePeriod} />
          <Segmented
            label="Courbe"
            className="toggle"
            options={[
              { value: "value", label: "Valeur" },
              { value: "perf", label: "Performance" },
            ]}
            value={view}
            onChange={(v) => {
              setView(v);
              setScrub(null);
              remember("view", v);
            }}
          />
        </div>
      </section>

      <dl className={`figures num${ov.loading ? " stale" : ""}`} aria-label="Chiffres clés">
        <div>
          <dt>Gain total</dt>
          <dd className={tone(o.total.gain)}>
            <Num value={o.total.gain} format={(v) => eur(v, { sign: true })} />
          </dd>
          <dd className="figure-sub">
            {o.total.invested > 0 && pct(o.total.gain / o.total.invested, { sign: true }) + (adjusted ? " du capital investi" : " des apports")}
          </dd>
        </div>
        <div>
          <dt>Rendement annuel</dt>
          <dd className={o.total.xirr === null ? "" : tone(o.total.xirr)}>
            {o.total.xirr === null ? "–" : <Num value={o.total.xirr} format={(v) => pct(v, { sign: true })} />}
          </dd>
          <dd className="figure-sub">selon tes apports</dd>
        </div>
        <div>
          <dt>Apports nets</dt>
          <dd>
            <Num value={o.total.contributed} format={(v) => eur(v)} />
          </dd>
          <dd className="figure-sub">versés moins retirés</dd>
          {adjusted && (
            <dd>
              <Adjusted amount={o.total.adjusted} />
            </dd>
          )}
        </div>
        <div>
          <dt>Revenus</dt>
          <dd>
            <Num value={o.total.rewards + o.total.dividends + o.total.interest} format={(v) => eur(v)} />
          </dd>
          <dd className="figure-sub">{incomeParts(o.total)}</dd>
        </div>
        <div>
          <dt>Frais payés</dt>
          <dd>
            <Num value={o.total.fees} format={(v) => eur(v)} />
          </dd>
          <dd className="figure-sub">{o.total.contributed > 0 && pct(o.total.fees / o.total.contributed) + " des apports"}</dd>
        </div>
      </dl>

      <div className="duo">
        <section aria-labelledby="h-coins">
          <div className="sec-head">
            <h2 id="h-coins">Actifs</h2>
            <a
              href="/actifs"
              className="more-link"
              onClick={(e) => {
                e.preventDefault();
                go("/actifs");
              }}
            >
              Tout voir
            </a>
          </div>
          {st.data ? (
            <CoinList coins={st.data.assets.filter((c) => c.held).slice(0, 6)} cash={st.data.cash} go={go} />
          ) : st.error ? (
            <Failed error={st.error} retry={st.reload} />
          ) : (
            <SkelRows rows={4} />
          )}
        </section>

        <aside className="side">
          <section aria-labelledby="h-alloc">
            <div className="sec-head">
              <h2 id="h-alloc">Répartition</h2>
              <Segmented
                label="Répartition par"
                className="toggle small"
                options={[
                  { value: "assets", label: "Actifs" },
                  { value: "classes", label: "Classes" },
                  { value: "sources", label: "Comptes" },
                ]}
                value={by}
                onChange={(v) => {
                  setBy(v);
                  remember("by", v);
                }}
              />
            </div>
            {pf.data ? (
              <>
                <Allocation
                  parts={
                    by === "assets"
                      ? [
                          ...pf.data.assets.map((a) => ({ key: a.id, label: label(a.asset), value: Number(a.value) })),
                          { key: "_cash", label: "Liquidités", value: Number(pf.data.total.cash) },
                        ]
                      : by === "classes"
                        ? Object.entries(o.classes).map(([k, v]) => ({ key: k, label: CLASS[k as keyof typeof CLASS], value: v ?? 0 }))
                        : pf.data.sources.map((s) => ({ key: s.id, label: s.name, value: Number(s.value) }))
                  }
                />
                {by !== "classes" && o.total.value > 0 && o.total.stable > 0 && (
                  <p className="stable num">
                    <span className="muted">Stablecoins</span>
                    <span>{pct(o.total.stable / o.total.value)}</span>
                  </p>
                )}
              </>
            ) : (
              <Skel h={120} r={12} w="100%" />
            )}
          </section>

          {scope !== "securities" && tax.data && tax.data.disposals.length > 0 && (
            <a
              href="/impots"
              className="tax-card"
              onClick={(e) => {
                e.preventDefault();
                go("/impots");
              }}
            >
              <span className="tax-card-text">
                <span className="tax-card-title">Déclaration {tax.data.year}</span>
                <span className="muted">Formulaire 2086</span>
              </span>
              <span className="tax-card-amount num">{tax.data.total.exempt ? "Exonéré" : eur(tax.data.total.tax)}</span>
              <svg viewBox="0 0 16 16" aria-hidden className="chev">
                <path d="m6 3 5 5-5 5" />
              </svg>
            </a>
          )}
        </aside>
      </div>

      {st.data && <StatsSection stats={st.data} go={go} />}

      <footer className="foot">
        <span>
          État au {date(o.at)}, {time(o.at)}
        </span>
        {o.unpriced.length > 0 && <span>Sans cours : {o.unpriced.map(label).join(", ")}</span>}
      </footer>
    </div>
  );
}

/** Cash held, as a row: one for all accounts, or the account's. */
function CashRow({ list, name }: { list: CashStat[]; name?: string }) {
  const value = list.reduce((s, c) => s + c.value, 0);
  const share = list.reduce((s, c) => s + c.share, 0);
  const foreign = list.flatMap((c) => c.held).filter((h) => h.symbol !== "EUR");
  const sub = name
    ? [foreign.length ? list[0].held.map((h) => (h.symbol === "EUR" ? eur(h.qty) : `${qty(h.qty)} ${h.symbol}`)).join(", ") : "Euros", pct(share)]
    : [list.length > 1 ? `${list.length} comptes` : list[0].account.name, pct(share)];
  return (
    <li>
      <div className="static">
        <Coin symbol="EUR" size={40} />
        <span className="coin-main">
          <span className="coin-name">{name ?? "Liquidités"}</span>
          <span className="coin-sub num">{sub.join(" · ")}</span>
        </span>
        <span className="coin-spark" />
        <span className="coin-num num">
          <span className="coin-value">{eur(value)}</span>
          {!name && <span className="coin-gain muted">{list.length > 1 ? "sur les comptes" : "Liquidités"}</span>}
        </span>
      </div>
    </li>
  );
}

/**
 * Coins as rows: icon, name and quantity, a month of price, value and gain.
 * cash: the accounts' cash, in one row among them by value, or one per
 * account (byAccount) after them.
 */
export function CoinList({ coins, cash = [], byAccount, go }: { coins: CoinStat[]; cash?: CashStat[]; byAccount?: boolean; go: (path: string) => void }) {
  const cashValue = cash.reduce((s, c) => s + c.value, 0);
  const oneRow = !byAccount && cash.length > 0 && cashValue >= 0.5;
  if (!coins.length && !oneRow && !(byAccount && cash.length)) return byAccount ? null : <p className="empty">Aucun actif.</p>;
  const at = oneRow ? coins.findIndex((c) => !c.held || c.value < cashValue) : -1;
  const cashAt = at < 0 ? coins.length : at;
  return (
    <ul className="coin-list">
      {byAccount && cash.map((c) => <CashRow key={c.account.id} list={[c]} name={c.account.name} />)}
      {coins.map((c, n) => {
        const g = c.held ? c.unrealized : c.gain;
        const row = (
          <li key={c.id}>
            <a
              href={"/actifs/" + c.id}
              onClick={(e) => {
                if (e.metaKey || e.ctrlKey) return;
                e.preventDefault();
                go("/actifs/" + c.id);
              }}
            >
              <Coin asset={c.asset} cls={c.class} size={40} />
              <span className="coin-main">
                <span className="coin-name">{label(c.asset)}</span>
                <span className="coin-sub num">{c.held ? `${qty(c.qty)}${c.asset.kind === "security" ? " parts" : ""} · ${pct(c.share)}` : "Plus détenu"}</span>
              </span>
              <span className="coin-spark">{c.spark && <Spark values={c.spark} />}</span>
              <span className="coin-num num">
                <span className="coin-value">{c.held ? eur(c.value) : eur(c.gain, { sign: true })}</span>
                {c.stable ? (
                  <span className="coin-gain muted">Stablecoin</span>
                ) : c.held ? (
                  <span className={`coin-gain ${tone(g)}`}>
                    {eur(g, { sign: true })}
                    {c.cost > 0 && ` · ${pct(g / c.cost, { sign: true })}`}
                  </span>
                ) : (
                  <span className="coin-gain muted">gain total</span>
                )}
              </span>
            </a>
          </li>
        );
        return n === cashAt && oneRow ? [<CashRow key="_cash" list={cash} />, row] : row;
      })}
      {oneRow && cashAt === coins.length && <CashRow list={cash} />}
    </ul>
  );
}

function StatsSection({ stats, go }: { stats: Stats; go: (path: string) => void }) {
  const months = stats.months;
  if (months.length < 2) return null;
  return (
    <section className="stats" aria-labelledby="h-stats">
      <h2 id="h-stats">Statistiques</h2>
      <div className="stat wide">
        <h3>Rendements mensuels</h3>
        <Heatmap months={months} />
      </div>
      <div className="stat">
        <h3>Apports par mois</h3>
        <MonthBars months={months} />
      </div>
      <div className="stat">
        <h3>D'où vient la valeur</h3>
        <SplitArea points={months} />
      </div>
      <div className="stat">
        <h3>Revenus cumulés</h3>
        <Income months={months} />
      </div>
      <div className="stat">
        <h3>Gain par actif</h3>
        <Ranking coins={stats.assets} onOpen={(s) => go("/actifs/" + s)} />
      </div>
    </section>
  );
}

/** What the income figure is made of, in a few words. */
function incomeParts(t: Overview["total"]): string {
  if (t.dividends > 0) return `dont ${eur(t.dividends)} de dividendes`;
  if (t.interest > 0) return `dont ${eur(t.interest)} d'intérêts`;
  return "staking compris";
}

/** Rewards, dividends and interest, received month after month. */
function Income({ months }: { months: Stats["months"] }) {
  const [at, setAt] = useState<number | null>(null);
  const month = (x: Stats["months"][number]) => x.rewards + x.dividends + x.interest;
  const { shown, cum } = useMemo(() => {
    let sum = 0;
    const all = months.map((x) => (sum += month(x)));
    const first = all.findIndex((v) => v > 0);
    const from = first < 0 ? months.length : Math.max(0, first - 1);
    return { shown: months.slice(from), cum: all.slice(from) };
  }, [months]);
  const t = useMemo(() => shown.map((x) => utc(x.m)), [shown]);
  if (shown.length < 2) return <p className="empty">Aucun revenu pour l'instant.</p>;
  const i = at ?? shown.length - 1;
  const m = shown[i];
  const last12 = months.slice(-12).reduce((s, x) => s + month(x), 0);
  const total = (k: "rewards" | "dividends" | "interest") => months.reduce((s, x) => s + x[k], 0);
  const split = (
    [
      ["Staking", total("rewards")],
      ["Dividendes", total("dividends")],
      ["Intérêts", total("interest")],
    ] as const
  ).filter(([, v]) => v > 0);
  return (
    <div>
      <LineChart
        t={t}
        y={cum}
        tone="ink"
        fromZero
        axis={eurShort}
        height={168}
        dataKey="income"
        label="Revenus cumulés, mois par mois"
        scrub={at}
        onScrub={setAt}
      />
      <p className="chart-note num" aria-live="polite">
        {at !== null ? (
          <>
            {monthLong(utc(m.m))} <b>{eur(cum[i])}</b>
            <span className="muted"> · {eur(month(m), { sign: true })} ce mois-là</span>
          </>
        ) : (
          <>
            <b>{eur(cum[i])}</b> reçus <span className="muted">· {eur(last12)} sur 12 mois</span>
          </>
        )}
      </p>
      {split.length > 1 && (
        <p className="chart-note income-split num">
          {split.map(([k, v]) => (
            <span key={k}>
              <span className="muted">{k}</span> {eur(v)}
            </span>
          ))}
        </p>
      )}
    </div>
  );
}

function HomeSkeleton() {
  return (
    <div className="home" aria-busy="true" aria-label="Chargement">
      <section className="hero">
        <Skel w={96} h={14} />
        <Skel w="min(380px, 80%)" h={56} r={12} className="skel-hero" />
        <Skel w={220} h={18} />
      </section>
      <div className="graph">
        <Skel w="100%" h={320} r={16} />
        <Slow />
      </div>
      <div className="figures">
        {Array.from({ length: 5 }, (_, n) => (
          <div key={n}>
            <Skel w={90} h={12} />
            <Skel w={110} h={22} />
          </div>
        ))}
      </div>
      <SkelRows rows={4} />
    </div>
  );
}
