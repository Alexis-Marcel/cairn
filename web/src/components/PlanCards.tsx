// The strategy's figures that are not about the model: on Accueil, where
// the money could lead and what could go wrong; on Impôts, the PEA, a sale's
// tax, the securities account, fees, the accounts to declare and the year.
// Each setting is changed where it is shown, and saved at once.

import { useMemo, useState } from "react";
import { get, message, send, useLoad, type Overview, type Plan, type Portfolio, type Stats, type TaxFR } from "../api";
import { date, eur, eurShort, monthLong, pct, tone } from "../format";
import { utc } from "../lib/chart";
import { abroad, concentration, crisisLoss, CRISES, ctoLines, custody, grow, model, peaFive, RATES, reached, type Model } from "../lib/strategy";
import { Coin } from "./Coin";
import { LineChart } from "./LineChart";
import { Segmented } from "./Segmented";

const YEAR = new Date().getFullYear();
const THIS_MONTH = new Date().toISOString().slice(0, 7);

export interface PlanData {
  plan: Plan;
  pf: Portfolio;
  ov: Overview;
  st: Stats;
  tax: TaxFR;
}

/** What the cards need, for the whole portfolio, and a way to change the plan. */
export function usePlanData(rev: number) {
  const [n, setN] = useState(0);
  const plan = useLoad("plan" + rev + "." + n, (s) => get<Plan>("plan", s));
  const pf = useLoad("portfolio" + rev, (s) => get<Portfolio>("portfolio", s));
  const ov = useLoad("overview" + rev, (s) => get<Overview>("overview", s));
  const st = useLoad("stats" + rev, (s) => get<Stats>("stats", s));
  const tax = useLoad("taxnow" + rev, (s) => get<TaxFR>("tax/fr?year=" + YEAR, s));
  const d = plan.data && pf.data && ov.data && st.data && tax.data ? { plan: plan.data, pf: pf.data, ov: ov.data, st: st.data, tax: tax.data } : null;
  const save = async (patch: Partial<Plan>) => {
    if (!plan.data) return;
    await send<Plan>("PUT", "plan", { ...plan.data, ...patch });
    setN((x) => x + 1);
  };
  return { d, save };
}

export const parseEur = (s: string) => {
  const n = Number(s.replace(/[\s€]/g, "").replace(",", "."));
  return Number.isFinite(n) && n > 0 ? n : 0;
};

/** A small number field saved when it loses the focus. */
function Inline({ id, label, value, unit, placeholder, onSave }: { id: string; label: string; value: number; unit?: string; placeholder?: string; onSave: (v: number) => Promise<void> }) {
  const [raw, setRaw] = useState(value ? String(value) : "");
  const [error, setError] = useState("");
  const commit = async () => {
    const v = Math.round(parseEur(raw));
    if (v === value) return;
    try {
      setError("");
      await onSave(v);
    } catch (e) {
      setError(message(e));
    }
  };
  return (
    <div className="field inline">
      <label htmlFor={id}>{label}</label>
      <span className="with-unit">
        <input
          id={id}
          type="text"
          inputMode="numeric"
          autoComplete="off"
          placeholder={placeholder ?? "0"}
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()}
          aria-invalid={!!error}
        />
        {unit && <span className="unit">{unit}</span>}
      </span>
      {error && <small className="form-error">{error}</small>}
    </div>
  );
}

// ---- Accueil: the goal, and the risks

/** Where the money could lead, the monthly plan, and what could go wrong. */
export function Outlook({ rev, go }: { rev: number; go: (path: string) => void }) {
  const { d, save } = usePlanData(rev);
  const m = useMemo(() => (d ? model(d.pf, d.plan, []) : null), [d]);
  if (!d || !m || m.invested <= 0) return null;
  return (
    <>
      <Goal d={d} save={save} />
      <section className="strat-sec" aria-labelledby="h-risk">
        <h2 id="h-risk">Risques</h2>
        <div className="strat-grid">
          <Crisis m={m} total={d.ov.total.value} />
          <Weight d={d} m={m} go={go} />
          <Custody d={d} save={save} />
        </div>
      </section>
    </>
  );
}

function Goal({ d, save }: { d: PlanData; save: (p: Partial<Plan>) => Promise<void> }) {
  const { plan } = d;
  const [at, setAt] = useState<number | null>(null);
  const monthly = plan.monthly;
  const start = d.ov.total.value;
  const goal = plan.goal;
  const end = goal && plan.goalYear > YEAR ? Math.min(plan.goalYear, YEAR + 40) : YEAR + 15;
  const months = (end - YEAR) * 12;
  const series = useMemo(() => RATES.map((r) => grow(start, monthly, r.rate, months)), [start, monthly, months]);
  const t = useMemo(() => {
    const now = new Date();
    return series[0].map((_, n) => Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + n, 1));
  }, [series]);
  const i = at ?? months;
  const when = (n: number) => (n < 0 ? "après " + end : n === 0 ? "déjà" : String(new Date(t[n]).getUTCFullYear()));
  return (
    <section className="strat-sec" aria-labelledby="h-goal">
      <div className="sec-head goal-head">
        <h2 id="h-goal">Objectif</h2>
        <div className="goal-fields">
          <Inline id="g-monthly" label="Par mois" unit="€" placeholder="Aucun" value={plan.monthly} onSave={(v) => save({ monthly: v })} />
          <Inline id="g-goal" label="Viser" unit="€" placeholder="Aucun" value={plan.goal} onSave={(v) => save({ goal: v, goalYear: v ? plan.goalYear || YEAR + 10 : 0 })} />
          {goal > 0 && <Inline id="g-year" label="en" value={plan.goalYear} onSave={(v) => save({ goalYear: v })} />}
        </div>
      </div>
      <LineChart
        t={t}
        y={series[1]}
        y2={series[0]}
        tone="ink"
        fromZero
        level={goal || undefined}
        axis={eurShort}
        height={200}
        dataKey={"proj" + months + monthly}
        label="Projection de la valeur, scénario moyen, et prudent en pointillés"
        scrub={at}
        onScrub={setAt}
      />
      <dl className="figures strat-figures num" aria-live="polite">
        {RATES.map((r, n) => (
          <div key={r.key}>
            <dt>
              {r.label} · {pct(r.rate, { rate: true })} par an
            </dt>
            <dd>{eur(series[n][i])}</dd>
            {goal > 0 && <dd className="figure-sub">objectif {when(reached(series[n], goal))}</dd>}
          </div>
        ))}
      </dl>
      {monthly > 0 && <Kept d={d} />}
      <p className="muted strat-note">
        En {monthLong(t[i])}, avec {eur(monthly)} par mois. Une illustration, pas une promesse.
      </p>
    </section>
  );
}

/** The months the plan was kept, as small bars against the amount planned. */
function Kept({ d }: { d: PlanData }) {
  const { plan, st } = d;
  const since = plan.since ?? THIS_MONTH;
  const by = new Map(st.months.map((m) => [m.m, m.in - m.out]));
  const months: string[] = [];
  for (let t = new Date(since + "-01T00:00:00Z"); t.toISOString().slice(0, 7) <= THIS_MONTH; t.setUTCMonth(t.getUTCMonth() + 1))
    months.push(t.toISOString().slice(0, 7));
  const shown = months.slice(-12);
  const done = shown.filter((m) => m !== THIS_MONTH);
  const kept = done.filter((m) => (by.get(m) ?? 0) >= plan.monthly * 0.9).length;
  const top = Math.max(plan.monthly, ...shown.map((m) => by.get(m) ?? 0)) || 1;
  if (shown.length < 3)
    return (
      <p className="kept-note num">
        Plan suivi depuis {monthLong(utc(since))} · {eur(Math.max(0, by.get(THIS_MONTH) ?? 0))} versés ce mois-ci
      </p>
    );
  return (
    <div className="kept">
      <ol className="plan-bars" aria-label="Versé chaque mois, contre le prévu">
        {shown.map((m) => {
          const v = Math.max(0, by.get(m) ?? 0);
          return (
            <li key={m} title={`${monthLong(utc(m))} : ${eur(v)}`}>
              <span className="plan-bar-col">
                <span className={v >= plan.monthly * 0.9 ? "plan-bar ok" : m === THIS_MONTH ? "plan-bar now" : "plan-bar"} style={{ height: `${(v / top) * 100}%` }} />
                <span className="plan-bar-aim" style={{ bottom: `${(plan.monthly / top) * 100}%` }} />
              </span>
              <span className="plan-bar-m">{new Intl.DateTimeFormat("fr-FR", { month: "narrow", timeZone: "UTC" }).format(utc(m))}</span>
              <span className="sr">
                {monthLong(utc(m))} : {eur(v)}
              </span>
            </li>
          );
        })}
      </ol>
      <p className="num">{done.length ? `${kept} mois tenus sur ${done.length}` : `Plan suivi depuis ${monthLong(utc(since))}`}</p>
    </div>
  );
}

function Crisis({ m, total }: { m: Model; total: number }) {
  const [key, setKey] = useState<(typeof CRISES)[number]["key"]>("crypto");
  const c = CRISES.find((x) => x.key === key)!;
  const loss = crisisLoss(m, c);
  return (
    <div className="stat">
      <h3>Test de crise</h3>
      <Segmented label="Scénario" className="chips" options={CRISES.map((x) => ({ value: x.key, label: x.label }))} value={key} onChange={setKey} />
      <p className="crisis num">
        <span className="down">{eur(loss)}</span>
        <span className="muted"> soit {pct(total > 0 ? loss / total : 0, { sign: true })}</span>
      </p>
      <p className="muted strat-note">
        Crypto {pct(c.crypto, { sign: true })}, bourse {pct(c.bourse, { sign: true })}. Il te resterait {eur(total + loss)}.
      </p>
    </div>
  );
}

function Weight({ d, m, go }: { d: PlanData; m: Model; go: (path: string) => void }) {
  const c = useMemo(() => concentration(d.pf, m.invested), [d.pf, m.invested]);
  if (!c.top) return null;
  return (
    <div className="stat">
      <h3>Concentration</h3>
      <dl className="mini num">
        <div>
          <dt>Plus grosse ligne</dt>
          <dd>
            {c.top.name} {pct(c.top.share)}
          </dd>
        </div>
        <div>
          <dt>5 plus grosses</dt>
          <dd>{pct(c.top5)}</dd>
        </div>
        <div>
          <dt>Lignes</dt>
          <dd>
            {c.lines.length}
            {c.small > 0 && <span className="muted"> dont {c.small} sous 1 %</span>}
          </dd>
        </div>
      </dl>
      <ul className="weights">
        {c.lines.slice(0, 5).map((l) => (
          <li key={l.id}>
            <a href={"/actifs/" + l.id} onClick={(e) => (e.preventDefault(), go("/actifs/" + l.id))}>
              <Coin asset={l.asset} cls={l.cls} size={20} />
              <span className="clip">{l.name}</span>
              <span className="num muted">{pct(c.share(l.value))}</span>
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Where the coins are kept; a tap on an account says whether it is yours. */
function Custody({ d, save }: { d: PlanData; save: (p: Partial<Plan>) => Promise<void> }) {
  const k = useMemo(() => custody(d.pf, d.plan), [d.pf, d.plan]);
  if (k.total < 1) return null;
  const toggle = (id: string) => {
    const own = new Set(k.list.filter((a) => a.own).map((a) => a.id));
    if (own.has(id)) own.delete(id);
    else own.add(id);
    save({ custody: [...own] });
  };
  return (
    <div className="stat">
      <h3>Garde des cryptos</h3>
      <p className="crisis num">
        {pct(k.mine / k.total)} <span className="muted">sur tes portefeuilles</span>
      </p>
      {k.btc > 0 && <p className="muted strat-note num">Bitcoin : {pct(k.btcMine / k.btc)} chez toi.</p>}
      <ul className="weights">
        {k.list.slice(0, 6).map((a) => (
          <li key={a.id}>
            <span className={a.own ? "own" : "platform"}>
              <button type="button" className="tag" aria-pressed={a.own} title="Changer" onClick={() => toggle(a.id)}>
                {a.own ? "À toi" : "Plateforme"}
              </button>
              <span className="clip">{a.name}</span>
              <span className="num muted">{eur(a.value)}</span>
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

// ---- Impôts: the year's landmarks

export function TaxCards({ rev }: { rev: number }) {
  const { d, save } = usePlanData(rev);
  if (!d) return null;
  return (
    <section className="strat-sec tax-cards" aria-labelledby="h-year">
      <h2 id="h-year">Ton année {YEAR}</h2>
      <Year d={d} />
      <div className="strat-grid">
        <Pea d={d} save={save} />
        <CryptoSale d={d} />
        <Cto d={d} />
        <Fees d={d} />
        <Abroad d={d} />
      </div>
    </section>
  );
}

const rateOf = (d: PlanData) => Number(d.tax.total.rate) || 0.314;

function Pea({ d, save }: { d: PlanData; save: (p: Partial<Plan>) => Promise<void> }) {
  const pea = d.plan.pea ? peaFive(d.plan.pea) : null;
  return (
    <div className="stat">
      <h3>PEA</h3>
      {pea && pea.days > 0 && (
        <p className="crisis num">
          {pea.days.toLocaleString("fr-FR")} <span className="muted">jours avant ses 5 ans</span>
        </p>
      )}
      {pea && pea.days <= 0 && <p className="crisis">Plus de 5 ans</p>}
      <p className="muted strat-note">
        {!pea
          ? "Sa date d'ouverture donne le compte à rebours de ses 5 ans."
          : pea.days > 0
            ? `Le ${date(pea.five.toISOString())}. Un retrait avant le ferme ; après, les gains ne paient plus que les prélèvements sociaux.`
            : "Les gains n'y paient plus que les prélèvements sociaux, et un retrait ne le ferme plus."}
      </p>
      <label className="field inline">
        <span>Ouvert le</span>
        <input type="date" defaultValue={d.plan.pea ?? ""} onBlur={(e) => e.target.value !== (d.plan.pea ?? "") && save({ pea: e.target.value || undefined })} />
      </label>
    </div>
  );
}

function CryptoSale({ d }: { d: PlanData }) {
  const rate = rateOf(d);
  const sold = Number(d.tax.total.disposals);
  const cr = d.pf.assets.filter((a) => a.class === "crypto" || a.class === "stable");
  const value = cr.reduce((s, a) => s + Number(a.value), 0);
  const cost = cr.reduce((s, a) => s + Number(a.cost), 0);
  const gainPart = value > 0 ? Math.max(0, 1 - cost / value) : 0;
  const [raw, setRaw] = useState("1000");
  const sale = parseEur(raw);
  const owed = sold + sale <= 305 ? 0 : sale * gainPart * rate;
  if (value < 1 && sold === 0) return null;
  return (
    <div className="stat">
      <h3>Vendre de la crypto</h3>
      <div className="meter" aria-hidden>
        <span style={{ width: `${Math.min(100, (sold / 305) * 100)}%` }} className={sold > 305 ? "over" : ""} />
      </div>
      <p className="muted strat-note num">
        {eur(sold)} vendus en {YEAR} {sold <= 305 ? "· exonéré jusqu'à 305 €" : "· seuil de 305 € dépassé"}
      </p>
      <div className="field inline">
        <label htmlFor="sale">Si je vends</label>
        <span className="with-unit">
          <input id="sale" type="text" inputMode="decimal" autoComplete="off" value={raw} onChange={(e) => setRaw(e.target.value)} />
          <span className="unit">€</span>
        </span>
      </div>
      <p className="num">
        Impôt ≈ <b>{eur(owed)}</b>
        <span className="muted"> · {pct(gainPart)} de gain, taxé à {pct(rate, { rate: true })}</span>
      </p>
      <p className="muted strat-note">Échanger une crypto contre une autre n'est pas imposé.</p>
    </div>
  );
}

function Cto({ d }: { d: PlanData }) {
  const rate = rateOf(d);
  const lines = useMemo(() => ctoLines(d.pf, rate), [d.pf, rate]);
  if (!lines.length) return null;
  const gains = lines.reduce((s, l) => s + l.gain, 0);
  const losses = lines.filter((l) => l.gain < 0).reduce((s, l) => s + l.gain, 0);
  return (
    <div className="stat">
      <h3>Compte-titres</h3>
      <dl className="mini num">
        <div>
          <dt>Tout vendre coûterait</dt>
          <dd>{eur(Math.max(0, gains) * rate)}</dd>
        </div>
        <div>
          <dt>Pertes latentes</dt>
          <dd className={tone(losses)}>{eur(losses)}</dd>
        </div>
      </dl>
      <ul className="weights">
        {lines.slice(0, 5).map((l) => (
          <li key={l.id}>
            <span>
              <Coin asset={l.asset} cls={l.cls} size={20} />
              <span className="clip">{l.name}</span>
              <span className={`num ${tone(l.gain)}`}>{eur(l.gain, { sign: true })}</span>
            </span>
          </li>
        ))}
      </ul>
      <p className="muted strat-note">Une perte vendue la même année qu'un gain réduit l'impôt ; elle se reporte 10 ans.</p>
    </div>
  );
}

function Fees({ d }: { d: PlanData }) {
  const first = d.ov.points[0]?.d;
  const years = first ? Math.max(1, (Date.now() - utc(first)) / (365.25 * 86400000)) : 1;
  return (
    <div className="stat">
      <h3>Frais payés</h3>
      <p className="crisis num">
        {eur(d.ov.total.fees)} <span className="muted">au total</span>
      </p>
      <p className="muted strat-note num">Environ {eur(d.ov.total.fees / years)} par an, sans les frais annuels des ETF.</p>
    </div>
  );
}

function Abroad({ d }: { d: PlanData }) {
  const list = useMemo(() => abroad(d.pf.sources.map((s) => s.name).concat(d.pf.assets.flatMap((a) => a.sources.map((s) => s.name)))), [d.pf]);
  if (!list.length) return null;
  return (
    <div className="stat">
      <h3>Comptes à déclarer</h3>
      <ul className="weights">
        {list.map((a) => (
          <li key={a.name}>
            <span>
              <span className="clip">{a.name}</span>
              <span className="num muted">{a.form}</span>
            </span>
          </li>
        ))}
      </ul>
      <p className="muted strat-note">Chaque printemps, avec ta déclaration de revenus. Un oubli coûte jusqu'à 750 € par compte.</p>
    </div>
  );
}

function Year({ d }: { d: PlanData }) {
  const ytd = d.ov.periods.find((p) => p.key === "YTD");
  const months = d.st.months.filter((x) => x.m.startsWith(String(YEAR)));
  const put = months.reduce((s, x) => s + x.in - x.out, 0);
  const earned = months.reduce((s, x) => s + x.rewards + x.dividends + x.interest, 0);
  return (
    <dl className="figures strat-figures num">
      <div>
        <dt>Gain depuis janvier</dt>
        <dd className={tone(ytd?.gain ?? 0)}>{eur(ytd?.gain ?? 0, { sign: true })}</dd>
        <dd className="figure-sub">{ytd?.pct != null ? pct(ytd.pct, { sign: true }) : " "}</dd>
      </div>
      <div>
        <dt>Apports nets</dt>
        <dd>{eur(put, { sign: true })}</dd>
      </div>
      <div>
        <dt>Revenus reçus</dt>
        <dd>{eur(earned)}</dd>
      </div>
      <div>
        <dt>Impôt crypto estimé</dt>
        <dd>{d.tax.total.exempt ? "Aucun" : eur(d.tax.total.tax)}</dd>
      </div>
    </dl>
  );
}
