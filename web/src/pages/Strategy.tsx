// Stratégie: the plan as a model portfolio, against what is held. What to
// do now comes first (the moves that cost no tax before the others), then
// the model line by line, where the next euros go, the monthly plan, where
// it could lead, the risks, the costs and taxes, and the year in short.

import { useMemo, useState } from "react";
import { get, useLoad, type Account, type Overview, type Plan, type Portfolio, type Stats, type TaxFR } from "../api";
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
  model,
  nextSplit,
  peaFive,
  RATES,
  reached,
  todo,
  type Model,
  type Row,
} from "../lib/strategy";

const YEAR = new Date().getFullYear();
const THIS_MONTH = new Date().toISOString().slice(0, 7);

interface Data {
  plan: Plan;
  pf: Portfolio;
  ov: Overview;
  st: Stats;
  tax: TaxFR;
  accounts: Account[];
}

export function Strategy({ rev, go }: { rev: number; go: (path: string) => void }) {
  const [planRev, setPlanRev] = useState(0);
  const plan = useLoad("plan" + rev + "." + planRev, (s) => get<Plan>("plan", s));
  const pf = useLoad("pf" + rev, (s) => get<Portfolio>("portfolio", s));
  const ov = useLoad("ov" + rev, (s) => get<Overview>("overview", s));
  const st = useLoad("st" + rev, (s) => get<Stats>("stats", s));
  const tax = useLoad("tax" + rev, (s) => get<TaxFR>("tax/fr?year=" + YEAR, s));
  const accounts = useLoad("acc" + rev, (s) => get<Account[]>("accounts", s));
  const [editing, setEditing] = useState(false);

  const failed = [plan, pf, ov, st, tax, accounts].find((l) => l.error && !l.data);
  if (failed) return <Failed error={failed.error!} retry={failed.reload} />;
  if (!plan.data || !pf.data || !ov.data || !st.data || !tax.data || !accounts.data) return <Loading />;
  const d: Data = { plan: plan.data, pf: pf.data, ov: ov.data, st: st.data, tax: tax.data, accounts: accounts.data };

  return (
    <div className="strategy">
      <div className="page-head">
        <h1>Stratégie</h1>
        <button type="button" className="btn" onClick={() => setEditing(true)}>
          {d.plan.set ? "Modifier" : "Créer mon modèle"}
        </button>
      </div>
      <Body d={d} go={go} edit={() => setEditing(true)} />
      {editing && (
        <PlanForm
          plan={d.plan}
          portfolio={d.pf}
          accounts={d.accounts}
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
  const m = useMemo(() => model(d.pf, d.plan, d.accounts), [d.pf, d.plan, d.accounts]);
  const rate = Number(d.tax.total.rate) || 0.314;
  const [amount, setAmount] = useState(() => String(Math.round(d.plan.monthly || m.waiting || 500)));
  const place = (v: number) => {
    setAmount(String(v));
    document.getElementById("h-next")?.scrollIntoView({ behavior: "smooth", block: "start" });
  };
  return (
    <>
      {d.plan.set ? (
        <>
          <Todos list={todo(m, d.plan, d.pf, rate)} place={place} />
          <Targets m={m} plan={d.plan} />
          <Next m={m} raw={amount} setRaw={setAmount} />
        </>
      ) : (
        <div className="plan-intro">
          <p>Décris ton portefeuille modèle : chaque actif, où l'acheter et sa part. Cairn te dit ensuite quoi faire et où placer chaque apport.</p>
          <button type="button" className="btn primary" onClick={edit}>
            Créer mon modèle
          </button>
        </div>
      )}
      <Monthly d={d} edit={edit} />
      <Projection d={d} />
      <section className="strat-sec" aria-labelledby="h-risk">
        <h2 id="h-risk">Risques</h2>
        <div className="strat-grid">
          <Crisis m={m} total={d.ov.total.value} />
          <Weight d={d} m={m} go={go} />
          <Custody d={d} edit={edit} />
        </div>
      </section>
      <Taxes d={d} rate={rate} edit={edit} />
      <Year d={d} m={m} />
    </>
  );
}

// ---- what to do

function Todos({ list, place }: { list: ReturnType<typeof todo>; place: (v: number) => void }) {
  return (
    <section className="strat-sec" aria-labelledby="h-todo">
      <h2 id="h-todo">À faire</h2>
      {list.length === 0 ? (
        <p className="strat-ok">
          <span className="dot" aria-hidden /> Rien à faire : ton portefeuille suit ton modèle.
        </p>
      ) : (
        <ol className="todos">
          {list.map((t) => (
            <li key={t.key} className={t.level ?? ""}>
              <span className="todo-text">{t.text}</span>
              {t.tax && (t.tax.free ? <span className="tag free">Sans impôt</span> : <span className="tag taxed num">Impôt ≈ {eur(t.tax.amount)}</span>)}
              {t.place !== undefined && (
                <button type="button" className="link-btn" onClick={() => place(t.place!)}>
                  Répartir
                </button>
              )}
            </li>
          ))}
        </ol>
      )}
      {list.length > 0 && <p className="muted strat-note">Chaque ligne disparaît d'elle-même une fois faite et synchronisée.</p>}
    </section>
  );
}

// ---- the model against what is held

function Gauge({ now, aim, off }: { now: number; aim: number; off: boolean }) {
  const scale = Math.max(now, aim, 0.0001) > 0.5 ? 1 : 0.5;
  return (
    <span className="gauge-track" aria-hidden>
      <span className={off ? "gauge-fill off" : "gauge-fill"} style={{ width: `${Math.min(100, (now / scale) * 100)}%` }} />
      <span className="gauge-aim" style={{ left: `${Math.min(100, (aim / scale) * 100)}%` }} />
    </span>
  );
}

const drift = (now: number, aim: number) => {
  const d = (now - aim) * 100;
  return Math.abs(d) < 0.5 ? "dans la cible" : `${d > 0 ? "+" : "−"}${Math.abs(d).toFixed(0)} pt${Math.abs(d) >= 1.5 ? "s" : ""}`;
};

function Targets({ m, plan }: { m: Model; plan: Plan }) {
  if (m.invested <= 0) return null;
  const share = (v: number) => v / m.invested;
  const band = plan.band / 100;
  const cNow = share(m.crypto);
  const outOf = (crypto: boolean) => m.out.filter((o) => o.crypto === crypto);
  const group = (title: string, crypto: boolean) => {
    const rows = m.rows.filter((r) => r.crypto === crypto);
    const out = outOf(crypto);
    const outValue = out.reduce((s, o) => s + o.value, 0);
    if (!rows.length && !outValue) return null;
    return (
      <tbody>
        <tr className="model-group">
          <th scope="rowgroup" colSpan={5}>
            {title}
          </th>
        </tr>
        {rows.map((r) => (
          <ModelRow key={r.id} r={r} now={share(r.value)} band={band} />
        ))}
        {outValue >= 1 && (
          <tr className="model-out">
            <th scope="row">
              <details>
                <summary>
                  Hors modèle <span className="muted">· {out.length} ligne{out.length > 1 ? "s" : ""}</span>
                </summary>
                <ul>
                  {out.map((o) => (
                    <li key={o.id}>
                      <span className="clip">{o.name}</span>
                      {o.sell && <span className="tag taxed">À vendre</span>}
                      <span className="num muted">{eur(o.value)}</span>
                    </li>
                  ))}
                </ul>
              </details>
            </th>
            <td className="model-gauge">
              <Gauge now={share(outValue)} aim={0} off={false} />
            </td>
            <td className="num">{pct(share(outValue))}</td>
            <td className="num muted">ne reçoit plus rien</td>
            <td className="num muted">{eur(outValue)}</td>
          </tr>
        )}
      </tbody>
    );
  };
  return (
    <section className="strat-sec" aria-labelledby="h-targets">
      <h2 id="h-targets">Mon modèle</h2>
      <div className="split-line">
        <span className="split-bar" aria-hidden>
          <span className="split-b" style={{ width: `${(1 - cNow) * 100}%` }} />
          <span className="split-aim" style={{ left: `${(1 - m.cryptoTarget) * 100}%` }} />
        </span>
        <p className="num">
          Bourse <b>{pct(1 - cNow)}</b> · Crypto <b>{pct(cNow)}</b>
          <span className={Math.abs(cNow - m.cryptoTarget) > band ? "gauge-drift off" : "muted"}>
            {" "}
            pour {pct(1 - m.cryptoTarget)} / {pct(m.cryptoTarget)} visés
          </span>
        </p>
      </div>
      <div className="scroll">
        <table className="model num">
          <thead>
            <tr>
              <th scope="col">Ligne</th>
              <th scope="col" className="model-gauge">
                <span className="sr">Aujourd'hui et cible</span>
              </th>
              <th scope="col">Aujourd'hui</th>
              <th scope="col">Cible</th>
              <th scope="col">Valeur</th>
            </tr>
          </thead>
          {group("Bourse", false)}
          {group("Crypto", true)}
        </table>
      </div>
      <p className="muted strat-note">
        Parts de tout ce qui est investi ; liquidités à part ({eur(m.waiting)}). Le hors modèle compte dans la bourse ou la crypto mais ne reçoit plus d'argent.
      </p>
    </section>
  );
}

function ModelRow({ r, now, band }: { r: Row; now: number; band: number }) {
  const off = Math.abs(now - r.target) > band / 2 && Math.abs(now - r.target) * r.target > 0;
  return (
    <tr>
      <th scope="row">
        <span className="model-name">
          <Coin asset={r.asset} cls={r.cls} size={22} />
          <span className="clip">{r.name}</span>
          {r.where && <span className="tag">{r.where}</span>}
        </span>
      </th>
      <td className="model-gauge">
        <Gauge now={now} aim={r.target} off={off} />
      </td>
      <td>{pct(now)}</td>
      <td>
        {pct(r.target)} <span className={off ? "gauge-drift off" : "gauge-drift"}>{drift(now, r.target)}</span>
      </td>
      <td className="muted">{eur(r.value)}</td>
    </tr>
  );
}

// ---- next contribution

const parseEur = (s: string) => {
  const n = Number(s.replace(/[\s€]/g, "").replace(",", "."));
  return Number.isFinite(n) && n > 0 ? n : 0;
};

function Next({ m, raw, setRaw }: { m: Model; raw: string; setRaw: (v: string) => void }) {
  const amount = parseEur(raw);
  const split = useMemo(() => nextSplit(m, amount), [m, amount]);
  const parts = m.rows.filter((r) => (split.get(r.id) ?? 0) > 0);
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
          {m.waiting >= 1 && Math.round(m.waiting) !== amount && (
            <button type="button" className="link-btn" onClick={() => setRaw(String(Math.round(m.waiting)))}>
              Mes liquidités : {eur(m.waiting)}
            </button>
          )}
        </div>
        {amount > 0 && (
          <ul className="next-split" aria-live="polite">
            {parts.map((r) => (
              <li key={r.id}>
                <Coin asset={r.asset} cls={r.cls} size={24} />
                <span className="next-name">
                  <span className="clip">{r.name}</span>
                  {r.where && <span className="muted">{r.where}</span>}
                </span>
                <span className="num">{eur(split.get(r.id) ?? 0)}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
      <p className="muted strat-note">Sans rien vendre : les lignes sous leur cible reçoivent d'abord.</p>
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

function Weight({ d, m, go }: { d: Data; m: Model; go: (path: string) => void }) {
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

function Year({ d, m }: { d: Data; m: Model }) {
  const ytd = d.ov.periods.find((p) => p.key === "YTD");
  const months = d.st.months.filter((x) => x.m.startsWith(String(YEAR)));
  const put = months.reduce((s, x) => s + x.in - x.out, 0);
  const earned = months.reduce((s, x) => s + x.rewards + x.dividends + x.interest, 0);
  const gap = m.invested > 0 ? Math.abs(m.crypto / m.invested - m.cryptoTarget) * 100 : 0;
  const checks = [
    { done: d.plan.set && gap <= d.plan.band, text: d.plan.set ? (gap <= d.plan.band ? "Crypto dans sa marge" : "Crypto hors de sa marge") : "Créer mon modèle" },
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
          <dd>{d.plan.set ? `${gap.toFixed(0)} pts` : "—"}</dd>
        </div>
      </dl>
      <ul className="todo">
        {checks.map((x) => (
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
