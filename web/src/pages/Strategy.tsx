// Stratégie: the plan against the portfolio. Where the money sits against
// its target, where the next euros go, the monthly plan, where it could
// lead, what a crisis would cost, what weighs too much, where the coins are
// kept, the costs, the taxes, and the year in short. The alerts come first.

import { useMemo, useState } from "react";
import { get, useLoad, type Overview, type Plan, type Portfolio, type Stats, type TaxFR } from "../api";
import { Coin } from "../components/Coin";
import { LineChart } from "../components/LineChart";
import { PlanForm } from "../components/PlanForm";
import { Segmented } from "../components/Segmented";
import { Failed, Loading } from "../components/State";
import { date, eur, eurShort, monthLong, pct, tone } from "../format";
import { utc } from "../lib/chart";
import {
  abroad,
  concentration,
  crisisLoss,
  CRISES,
  ctoLines,
  custody,
  grow,
  nextSplit,
  peaFive,
  position,
  RATES,
  reached,
  SLEEVE_NAME,
  SLEEVES,
  targets,
  type Position,
} from "../lib/strategy";

const YEAR = new Date().getFullYear();
const THIS_MONTH = new Date().toISOString().slice(0, 7);

interface Data {
  plan: Plan;
  pf: Portfolio;
  ov: Overview;
  st: Stats;
  tax: TaxFR;
}

export function Strategy({ rev, go }: { rev: number; go: (path: string) => void }) {
  const [planRev, setPlanRev] = useState(0);
  const plan = useLoad("plan" + rev + "." + planRev, (s) => get<Plan>("plan", s));
  const pf = useLoad("pf" + rev, (s) => get<Portfolio>("portfolio", s));
  const ov = useLoad("ov" + rev, (s) => get<Overview>("overview", s));
  const st = useLoad("st" + rev, (s) => get<Stats>("stats", s));
  const tax = useLoad("tax" + rev, (s) => get<TaxFR>("tax/fr?year=" + YEAR, s));
  const [editing, setEditing] = useState(false);

  const all = [plan, pf, ov, st, tax];
  const failed = all.find((l) => l.error && !l.data);
  if (failed) return <Failed error={failed.error!} retry={failed.reload} />;
  if (!plan.data || !pf.data || !ov.data || !st.data || !tax.data) return <Loading />;
  const d: Data = { plan: plan.data, pf: pf.data, ov: ov.data, st: st.data, tax: tax.data };

  return (
    <div className="strategy">
      <div className="page-head">
        <h1>Stratégie</h1>
        <button type="button" className="btn" onClick={() => setEditing(true)}>
          {d.plan.set ? "Modifier" : "Fixer ma cible"}
        </button>
      </div>
      <Body d={d} go={go} edit={() => setEditing(true)} />
      {editing && (
        <PlanForm
          plan={d.plan}
          wallets={custody(d.pf, d.plan).list}
          onClose={() => setEditing(false)}
          onSaved={() => {
            setEditing(false);
            setPlanRev((n) => n + 1);
          }}
        />
      )}
    </div>
  );
}

function Body({ d, go, edit }: { d: Data; go: (path: string) => void; edit: () => void }) {
  const pos = useMemo(() => position(d.pf), [d.pf]);
  const rate = Number(d.tax.total.rate) || 0.314;
  return (
    <>
      {!d.plan.set && (
        <div className="plan-intro">
          <p>Fixe ta répartition cible : Cairn te dit ensuite où placer chaque apport et te prévient quand tu t'en écartes.</p>
          <button type="button" className="btn primary" onClick={edit}>
            Fixer ma cible
          </button>
        </div>
      )}
      {d.plan.set && <Alerts d={d} pos={pos} />}
      <Targets plan={d.plan} pos={pos} />
      <Next plan={d.plan} pos={pos} />
      <Monthly d={d} edit={edit} />
      <Projection d={d} />
      <section className="strat-sec" aria-labelledby="h-risk">
        <h2 id="h-risk">Risques</h2>
        <div className="strat-grid">
          <Crisis pos={pos} total={d.ov.total.value} />
          <Weight d={d} pos={pos} go={go} />
          <Custody d={d} edit={edit} />
        </div>
      </section>
      <Taxes d={d} rate={rate} edit={edit} />
      <Year d={d} pos={pos} />
    </>
  );
}

// ---- alerts

interface Alert {
  level: "warn" | "info" | "good";
  text: React.ReactNode;
}

function alerts(d: Data, pos: Position): Alert[] {
  const out: Alert[] = [];
  const { plan } = d;
  if (pos.invested > 0) {
    const share = pos.crypto / pos.invested;
    const aim = plan.crypto / 100;
    const band = plan.band / 100;
    if (share > aim + band)
      out.push({
        level: "warn",
        text: `La crypto pèse ${pct(share)}, au-delà de ${pct(aim + band)} : revends-en ${eur((share - aim) * pos.invested)} pour revenir à ${pct(aim)}.`,
      });
    else if (share < aim - band)
      out.push({
        level: "warn",
        text: `La crypto pèse ${pct(share)}, sous ${pct(Math.max(0, aim - band))} : tes prochains apports vont en priorité au bitcoin.`,
      });
    const c = concentration(d.pf, pos.invested);
    for (const l of c.lines) {
      if ((l.sleeve === "stock" || l.sleeve === "alt") && c.share(l.value) > plan.line / 100)
        out.push({ level: "warn", text: `${l.name} pèse ${pct(c.share(l.value))} : plus que ta limite de ${plan.line} % par ligne.` });
    }
  }
  if (pos.waiting >= 300 && pos.waiting > 0.03 * (pos.invested + pos.waiting))
    out.push({ level: "info", text: `${eur(pos.waiting)} de liquidités attendent d'être placés.` });
  const k = custody(d.pf, plan);
  if (k.btc - k.btcMine >= 1000) out.push({ level: "info", text: `${eur(k.btc - k.btcMine)} de bitcoin restent sur des plateformes.` });
  if (plan.pea) {
    const { five, days } = peaFive(plan.pea);
    if (days > 0 && days <= 90) out.push({ level: "good", text: `Ton PEA a 5 ans le ${date(five.toISOString())} : ne retire rien d'ici là.` });
  }
  if (!d.tax.total.exempt && Number(d.tax.total.disposals) > 0)
    out.push({ level: "info", text: `Tes ventes de crypto de ${YEAR} dépassent 305 € : environ ${eur(d.tax.total.tax)} d'impôt à prévoir.` });
  return out;
}

function Alerts({ d, pos }: { d: Data; pos: Position }) {
  const list = alerts(d, pos);
  if (!list.length)
    return (
      <p className="strat-ok">
        <span className="dot" aria-hidden /> Tout est dans ta cible.
      </p>
    );
  return (
    <ul className="strat-alerts" aria-label="Alertes">
      {list.map((a, n) => (
        <li key={n} className={a.level}>
          {a.text}
        </li>
      ))}
    </ul>
  );
}

// ---- targets

function Gauge({ label, now, aim, band, value }: { label: string; now: number; aim: number; band: number; value: number }) {
  const drift = (now - aim) * 100;
  const off = Math.abs(drift) > band;
  return (
    <li className="gauge">
      <span className="gauge-name">{label}</span>
      <span className="gauge-track" aria-hidden>
        <span className={off ? "gauge-fill off" : "gauge-fill"} style={{ width: `${Math.min(100, now * 100)}%` }} />
        <span className="gauge-aim" style={{ left: `${aim * 100}%` }} />
      </span>
      <span className="num gauge-now">{pct(now)}</span>
      <span className={`num gauge-drift ${off ? "off" : ""}`}>
        {Math.abs(drift) < 0.5 ? "dans la cible" : `${drift > 0 ? "+" : "−"}${Math.abs(drift).toFixed(0)} pt${Math.abs(drift) >= 1.5 ? "s" : ""}`}
        <span className="sr"> par rapport à la cible de {pct(aim)}</span>
      </span>
      <span className="num muted gauge-value">{eur(value)}</span>
    </li>
  );
}

function Targets({ plan, pos }: { plan: Plan; pos: Position }) {
  if (pos.invested <= 0) return null;
  const share = (v: number, of: number) => (of > 0 ? v / of : 0);
  const v = pos.value;
  return (
    <section className="strat-sec" aria-labelledby="h-targets">
      <h2 id="h-targets">Répartition</h2>
      <p className="muted strat-lede">
        La barre, c'est aujourd'hui ; le trait, ta cible. Liquidités à part : {eur(pos.waiting)}.
      </p>
      <ul className="gauges">
        <Gauge label="Bourse" now={share(pos.bourse, pos.invested)} aim={1 - plan.crypto / 100} band={plan.band} value={pos.bourse} />
        <Gauge label="Crypto" now={share(pos.crypto, pos.invested)} aim={plan.crypto / 100} band={plan.band} value={pos.crypto} />
      </ul>
      <div className="gauge-pair">
        {pos.bourse > 0 && (
          <div>
            <h3>Dans la bourse</h3>
            <ul className="gauges small">
              <Gauge label="ETF" now={share(v.etf, pos.bourse)} aim={plan.etf / 100} band={plan.band} value={v.etf} />
              <Gauge label="Actions" now={share(v.stock, pos.bourse)} aim={1 - plan.etf / 100} band={plan.band} value={v.stock} />
            </ul>
          </div>
        )}
        {pos.crypto > 0 && (
          <div>
            <h3>Dans la crypto</h3>
            <ul className="gauges small">
              <Gauge label="Bitcoin" now={share(v.btc, pos.crypto)} aim={plan.btc / 100} band={plan.band} value={v.btc} />
              <Gauge label="Ether" now={share(v.eth, pos.crypto)} aim={plan.eth / 100} band={plan.band} value={v.eth} />
              <Gauge label="Autres" now={share(v.alt, pos.crypto)} aim={Math.max(0, 1 - (plan.btc + plan.eth) / 100)} band={plan.band} value={v.alt} />
            </ul>
          </div>
        )}
      </div>
    </section>
  );
}

// ---- next contribution

const parseEur = (s: string) => {
  const n = Number(s.replace(/[\s€]/g, "").replace(",", "."));
  return Number.isFinite(n) && n > 0 ? n : 0;
};

function Next({ plan, pos }: { plan: Plan; pos: Position }) {
  const suggested = Math.round(plan.monthly || pos.waiting || 500);
  const [raw, setRaw] = useState(String(suggested));
  const amount = parseEur(raw);
  const split = useMemo(() => nextSplit(pos, plan, amount), [pos, plan, amount]);
  const parts = SLEEVES.filter((s) => split[s] > 0);
  return (
    <section className="strat-sec" aria-labelledby="h-next">
      <h2 id="h-next">Prochain apport</h2>
      <div className="next">
        <div className="field next-amount">
          <label htmlFor="next-amount">Montant à placer</label>
          <span className="with-unit">
            <input id="next-amount" type="text" inputMode="decimal" autoComplete="off" value={raw} onChange={(e) => setRaw(e.target.value)} />
            <span className="unit">€</span>
          </span>
          {pos.waiting >= 1 && Math.round(pos.waiting) !== amount && (
            <button type="button" className="link-btn" onClick={() => setRaw(String(Math.round(pos.waiting)))}>
              Mes liquidités : {eur(pos.waiting)}
            </button>
          )}
        </div>
        {amount > 0 && (
          <ul className="next-split" aria-live="polite">
            {parts.map((s) => (
              <li key={s}>
                <span className={`sw s-${s}`} aria-hidden />
                <span>{SLEEVE_NAME[s]}</span>
                <span className="num">{eur(split[s])}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
      <p className="muted strat-note">Sans rien vendre : ce qui est sous sa cible reçoit d'abord.</p>
    </section>
  );
}

// ---- monthly plan

function Monthly({ d, edit }: { d: Data; edit: () => void }) {
  const { plan, st } = d;
  const btc = st.assets.find((c) => c.asset.symbol === "BTC" && c.held);
  const btcLine = btc && btc.avgCost > 0 && (
    <p className="strat-note num">
      Bitcoin : prix moyen {eur(btc.avgCost)}, cours {eur(btc.price)}{" "}
      <span className={tone(btc.price - btc.avgCost)}>{pct(btc.price / btc.avgCost - 1, { sign: true })}</span>
    </p>
  );
  if (!plan.monthly)
    return (
      <section className="strat-sec" aria-labelledby="h-monthly">
        <h2 id="h-monthly">Plan mensuel</h2>
        <p className="muted">
          Pas encore d'apport mensuel.{" "}
          <button type="button" className="link-btn" onClick={edit}>
            En prévoir un
          </button>
        </p>
        {btcLine}
      </section>
    );
  const since = plan.since ?? THIS_MONTH;
  const by = new Map(st.months.map((m) => [m.m, m.in - m.out]));
  const months: string[] = [];
  for (let t = new Date(since + "-01T00:00:00Z"); t.toISOString().slice(0, 7) <= THIS_MONTH; t.setUTCMonth(t.getUTCMonth() + 1))
    months.push(t.toISOString().slice(0, 7));
  const shown = months.slice(-12);
  const done = shown.filter((m) => m !== THIS_MONTH);
  const kept = done.filter((m) => (by.get(m) ?? 0) >= plan.monthly * 0.9).length;
  const paid = shown.reduce((s, m) => s + (by.get(m) ?? 0), 0);
  const top = Math.max(plan.monthly, ...shown.map((m) => by.get(m) ?? 0)) || 1;
  return (
    <section className="strat-sec" aria-labelledby="h-monthly">
      <h2 id="h-monthly">Plan mensuel</h2>
      <dl className="figures strat-figures num">
        <div>
          <dt>Prévu</dt>
          <dd>{eur(plan.monthly)} / mois</dd>
        </div>
        <div>
          <dt>Mois tenus</dt>
          <dd>{done.length ? `${kept} sur ${done.length}` : "—"}</dd>
        </div>
        <div>
          <dt>Versé depuis {monthLong(utc(since))}</dt>
          <dd>{eur(paid)}</dd>
        </div>
      </dl>
      <ol className="plan-bars" aria-label="Versé chaque mois, contre le prévu">
        {shown.map((m) => {
          const v = Math.max(0, by.get(m) ?? 0);
          const ok = v >= plan.monthly * 0.9;
          return (
            <li key={m} title={`${monthLong(utc(m))} : ${eur(v)}`}>
              <span className="plan-bar-col">
                <span className={ok ? "plan-bar ok" : m === THIS_MONTH ? "plan-bar now" : "plan-bar"} style={{ height: `${(v / top) * 100}%` }} />
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
      {btcLine}
    </section>
  );
}

// ---- projection

function Projection({ d }: { d: Data }) {
  const { plan } = d;
  const [raw, setRaw] = useState(plan.monthly ? String(plan.monthly) : "");
  const [at, setAt] = useState<number | null>(null);
  const monthly = parseEur(raw);
  const start = d.ov.total.value;
  const goal = plan.goal;
  const end = goal && plan.goalYear > YEAR ? Math.min(plan.goalYear, YEAR + 40) : YEAR + 15;
  const months = (end - YEAR) * 12;
  const series = useMemo(() => RATES.map((r) => grow(start, monthly, r.rate, months)), [start, monthly, months]);
  const t = useMemo(() => {
    const now = new Date();
    return series[0].map((_, n) => Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + n, 1));
  }, [series]);
  if (start <= 0 && monthly <= 0) return null;
  const i = at ?? months;
  const when = (n: number) => (n < 0 ? "après " + end : n === 0 ? "déjà" : String(new Date(t[n]).getUTCFullYear()));
  return (
    <section className="strat-sec" aria-labelledby="h-proj">
      <div className="sec-head">
        <h2 id="h-proj">Projection</h2>
        <div className="field inline">
          <label htmlFor="proj-monthly">Par mois</label>
          <span className="with-unit">
            <input id="proj-monthly" type="text" inputMode="decimal" autoComplete="off" placeholder="0" value={raw} onChange={(e) => setRaw(e.target.value)} />
            <span className="unit">€</span>
          </span>
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
        height={220}
        dataKey={"proj" + months}
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
      <p className="muted strat-note">
        En {monthLong(t[i])}, avec {eur(monthly)} par mois. Une illustration, pas une promesse : les marchés ne montent pas en ligne droite.
      </p>
    </section>
  );
}

// ---- risks

function Crisis({ pos, total }: { pos: Position; total: number }) {
  const [key, setKey] = useState<(typeof CRISES)[number]["key"]>("crypto");
  const c = CRISES.find((x) => x.key === key)!;
  const loss = crisisLoss(pos, c);
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

function Weight({ d, pos, go }: { d: Data; pos: Position; go: (path: string) => void }) {
  const c = useMemo(() => concentration(d.pf, pos.invested), [d.pf, pos.invested]);
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
      <p className="muted strat-note">Un ETF compte pour une ligne, même s'il contient des centaines d'entreprises.</p>
    </div>
  );
}

function Custody({ d, edit }: { d: Data; edit: () => void }) {
  const k = useMemo(() => custody(d.pf, d.plan), [d.pf, d.plan]);
  if (k.total < 1) return null;
  return (
    <div className="stat">
      <h3>Garde des cryptos</h3>
      <p className="crisis num">
        {pct(k.total > 0 ? k.mine / k.total : 0)} <span className="muted">sur tes portefeuilles</span>
      </p>
      {k.btc > 0 && <p className="muted strat-note num">Bitcoin : {pct(k.btcMine / k.btc)} chez toi.</p>}
      <ul className="weights">
        {k.list.slice(0, 6).map((a) => (
          <li key={a.id}>
            <span className={a.own ? "own" : "platform"}>
              <span className="tag">{a.own ? "À toi" : "Plateforme"}</span>
              <span className="clip">{a.name}</span>
              <span className="num muted">{eur(a.value)}</span>
            </span>
          </li>
        ))}
      </ul>
      <button type="button" className="link-btn" onClick={edit}>
        Choisir mes portefeuilles
      </button>
    </div>
  );
}

// ---- costs and taxes

function Taxes({ d, rate, edit }: { d: Data; rate: number; edit: () => void }) {
  const lines = useMemo(() => ctoLines(d.pf, rate), [d.pf, rate]);
  const accounts = useMemo(() => abroad(d.pf.sources.map((s) => s.name).concat(d.pf.assets.flatMap((a) => a.sources.map((s) => s.name)))), [d.pf]);
  const gains = lines.reduce((s, l) => s + l.gain, 0);
  const losses = lines.filter((l) => l.gain < 0).reduce((s, l) => s + l.gain, 0);
  const first = d.ov.points[0]?.d;
  const years = first ? Math.max(1, (Date.now() - utc(first)) / (365.25 * 86400000)) : 1;
  const sold = Number(d.tax.total.disposals);

  // Crypto: the share of a sale that is gain, as the 2086 counts it.
  const cr = d.pf.assets.filter((a) => a.class === "crypto" || a.class === "stable");
  const crValue = cr.reduce((s, a) => s + Number(a.value), 0);
  const crCost = cr.reduce((s, a) => s + Number(a.cost), 0);
  const gainPart = crValue > 0 ? Math.max(0, 1 - crCost / crValue) : 0;
  const [raw, setRaw] = useState("1000");
  const sale = parseEur(raw);
  const yearSales = sold + sale;
  const cryptoTax = yearSales <= 305 ? 0 : sale * gainPart * rate;

  const pea = d.plan.pea ? peaFive(d.plan.pea) : null;
  return (
    <section className="strat-sec" aria-labelledby="h-tax">
      <h2 id="h-tax">Frais et impôts</h2>
      <div className="strat-grid">
        <div className="stat">
          <h3>PEA</h3>
          {pea ? (
            pea.days > 0 ? (
              <>
                <p className="crisis num">
                  {pea.days.toLocaleString("fr-FR")} <span className="muted">jours avant ses 5 ans</span>
                </p>
                <p className="muted strat-note">
                  Le {date(pea.five.toISOString())}. Un retrait avant le ferme ; après, les gains ne paient plus que les prélèvements sociaux.
                </p>
              </>
            ) : (
              <>
                <p className="crisis">Plus de 5 ans</p>
                <p className="muted strat-note">Les gains n'y paient plus que les prélèvements sociaux. Les retraits ne le ferment plus.</p>
              </>
            )
          ) : (
            <p className="muted">
              <button type="button" className="link-btn" onClick={edit}>
                Indiquer son ouverture
              </button>{" "}
              pour suivre ses 5 ans.
            </p>
          )}
        </div>

        <div className="stat">
          <h3>Vendre de la crypto</h3>
          <div className="meter" aria-hidden>
            <span style={{ width: `${Math.min(100, (sold / 305) * 100)}%` }} className={sold > 305 ? "over" : ""} />
          </div>
          <p className="muted strat-note num">
            {eur(sold)} vendus en {YEAR} {sold <= 305 ? `· exonéré jusqu'à 305 €` : "· seuil de 305 € dépassé"}
          </p>
          <div className="field inline">
            <label htmlFor="sale">Si je vends</label>
            <span className="with-unit">
              <input id="sale" type="text" inputMode="decimal" autoComplete="off" value={raw} onChange={(e) => setRaw(e.target.value)} />
              <span className="unit">€</span>
            </span>
          </div>
          <p className="num">
            Impôt ≈ <b>{eur(cryptoTax)}</b>
            <span className="muted"> · {pct(gainPart)} de gain, taxé à {pct(rate, { rate: true })}</span>
          </p>
          <p className="muted strat-note">Échanger une crypto contre une autre n'est pas imposé.</p>
        </div>

        {lines.length > 0 && (
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
            <p className="muted strat-note">Vendre une ligne en perte la même année qu'une en gain efface l'impôt ; une perte se reporte 10 ans.</p>
          </div>
        )}

        <div className="stat">
          <h3>Frais payés</h3>
          <p className="crisis num">
            {eur(d.ov.total.fees)} <span className="muted">au total</span>
          </p>
          <p className="muted strat-note num">Environ {eur(d.ov.total.fees / years)} par an, sans les frais annuels des ETF.</p>
        </div>

        {accounts.length > 0 && (
          <div className="stat">
            <h3>Comptes à déclarer</h3>
            <ul className="weights">
              {accounts.map((a) => (
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
        )}
      </div>
    </section>
  );
}

// ---- the year in short

function Year({ d, pos }: { d: Data; pos: Position }) {
  const ytd = d.ov.periods.find((p) => p.key === "YTD");
  const months = d.st.months.filter((m) => m.m.startsWith(String(YEAR)));
  const put = months.reduce((s, m) => s + m.in - m.out, 0);
  const earned = months.reduce((s, m) => s + m.rewards + m.dividends + m.interest, 0);
  const t = targets(d.plan);
  const drift = pos.invested > 0 ? Math.abs(pos.crypto / pos.invested - (t.btc + t.eth + t.alt)) * 100 : 0;
  const todo = [
    { done: !d.plan.set || drift <= d.plan.band, text: d.plan.set ? (drift <= d.plan.band ? "Répartition dans la cible" : "Répartition à corriger") : "Fixer une cible" },
    { done: !!d.plan.pea, text: d.plan.pea ? "Date du PEA connue" : "Indiquer l'ouverture du PEA" },
    { done: d.tax.total.exempt, text: d.tax.total.exempt ? "Pas d'impôt crypto cette année" : `Déclarer la 2086 (≈ ${eur(d.tax.total.tax)})` },
    { done: false, text: "Déclarer les comptes à l'étranger" },
  ];
  return (
    <section className="strat-sec" aria-labelledby="h-year">
      <h2 id="h-year">Bilan {YEAR}</h2>
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
          <dt>Écart crypto</dt>
          <dd>{d.plan.set ? `${drift.toFixed(0)} pts` : "—"}</dd>
        </div>
      </dl>
      <ul className="todo">
        {todo.map((x) => (
          <li key={x.text} className={x.done ? "ok" : ""}>
            <span className="box" aria-hidden />
            {x.text}
            <span className="sr">{x.done ? " : fait" : " : à faire"}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

