// Stratégie: what to do now, the model portfolio against what is held
// (changed in place), and where the next euros go. Nothing else: where the
// money could lead and the risks are on Accueil, the PEA and taxes on Impôts.

import { useMemo, useState } from "react";
import { get, message, send, useLoad, type Account, type AssetRef, type Plan, type PlanLine, type Portfolio, type TaxFR } from "../api";
import { ISIN, label } from "../assets";
import { Coin } from "../components/Coin";
import { parseEur } from "../components/PlanCards";
import { AssetPicker } from "../components/Pickers";
import { Failed, Loading } from "../components/State";
import { eur, num, pct } from "../format";
import { model, nextSplit, todo, type Model, type Row } from "../lib/strategy";

const YEAR = new Date().getFullYear();

interface Data {
  plan: Plan;
  pf: Portfolio;
  tax: TaxFR;
  accounts: Account[];
}

export function Strategy({ rev }: { rev: number }) {
  const [n, setN] = useState(0);
  const plan = useLoad("plan" + rev + "." + n, (s) => get<Plan>("plan", s));
  const pf = useLoad("portfolio" + rev, (s) => get<Portfolio>("portfolio", s));
  const tax = useLoad("taxnow" + rev, (s) => get<TaxFR>("tax/fr?year=" + YEAR, s));
  const accounts = useLoad("acc" + rev, (s) => get<Account[]>("accounts", s));
  const failed = [plan, pf, tax, accounts].find((l) => l.error && !l.data);
  if (failed) return <Failed error={failed.error!} retry={failed.reload} />;
  if (!plan.data || !pf.data || !tax.data || !accounts.data) return <Loading />;
  const d: Data = { plan: plan.data, pf: pf.data, tax: tax.data, accounts: accounts.data };
  return (
    <div className="strategy">
      <div className="page-head">
        <h1>Stratégie</h1>
      </div>
      <Body d={d} saved={() => setN((x) => x + 1)} />
    </div>
  );
}

function Body({ d, saved }: { d: Data; saved: () => void }) {
  const m = useMemo(() => model(d.pf, d.plan, d.accounts), [d.pf, d.plan, d.accounts]);
  const rate = Number(d.tax.total.rate) || 0.314;
  const [editing, setEditing] = useState<Draft[] | null>(null);
  const [amount, setAmount] = useState(() => String(Math.round(d.plan.monthly || m.waiting || 500)));
  const place = (v: number) => {
    setAmount(String(v));
    document.getElementById("h-next")?.scrollIntoView({ behavior: "smooth", block: "start" });
  };
  const edit = (from: Draft[]) => setEditing(from);

  if (editing)
    return (
      <Editor
        d={d}
        start={editing}
        onDone={(ok) => {
          setEditing(null);
          if (ok) saved();
        }}
      />
    );
  if (!d.plan.set)
    return (
      <div className="plan-intro">
        <p>Décris ton portefeuille modèle : chaque actif, où l'acheter et sa part. Cairn te dit ensuite quoi faire et où placer chaque euro.</p>
        <div className="actions">
          <button type="button" className="btn primary" onClick={() => edit(fromHoldings(m))}>
            Partir de mon portefeuille
          </button>
          <button type="button" className="btn" onClick={() => edit([])}>
            Partir de zéro
          </button>
        </div>
      </div>
    );
  return (
    <>
      <Todos list={todo(m, d.plan, d.pf, rate)} place={place} />
      <Targets m={m} plan={d.plan} onEdit={() => edit(fromPlan(d.plan, m))} />
      <Next m={m} raw={amount} setRaw={setAmount} />
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
    </section>
  );
}

// ---- the model against what is held

function Gauge({ now, aim, off }: { now: number; aim: number; off: boolean }) {
  const scale = Math.max(now, aim) > 0.5 ? 1 : 0.5;
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

function Targets({ m, plan, onEdit }: { m: Model; plan: Plan; onEdit: () => void }) {
  if (m.invested <= 0) return null;
  const share = (v: number) => v / m.invested;
  const band = plan.band / 100;
  const cNow = share(m.crypto);
  const group = (title: string, crypto: boolean) => {
    const rows = m.rows.filter((r) => r.crypto === crypto);
    const out = m.out.filter((o) => o.crypto === crypto);
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
            <td>{pct(share(outValue))}</td>
            <td className="muted">ne reçoit plus rien</td>
            <td className="muted">{eur(outValue)}</td>
          </tr>
        )}
      </tbody>
    );
  };
  return (
    <section className="strat-sec" aria-labelledby="h-targets">
      <div className="sec-head">
        <h2 id="h-targets">Mon modèle</h2>
        <button type="button" className="btn" onClick={onEdit}>
          Modifier
        </button>
      </div>
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
      <p className="muted strat-note">Parts de tout ce qui est investi ; liquidités à part ({eur(m.waiting)}).</p>
    </section>
  );
}

function ModelRow({ r, now, band }: { r: Row; now: number; band: number }) {
  const off = Math.abs(now - r.target) > band / 2;
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

// ---- the model, changed in place

interface Draft {
  key: number;
  id: string;
  name: string;
  account: string;
  share: string;
}

let nextKey = 0;
const blank = (): Draft => ({ key: nextKey++, id: "", name: "", account: "", share: "" });
const fmtShare = (v: number) => String(Math.round(v * 10) / 10).replace(".", ",");
const parseShare = (s: string) => {
  const x = Number(s.replace(/[\s%]/g, "").replace(",", "."));
  return s.trim() && Number.isFinite(x) ? x : NaN;
};
const biggest = (h: Portfolio["assets"][number]) => [...h.sources].sort((a, b) => num(b.qty) - num(a.qty))[0]?.id ?? "";

function fromPlan(plan: Plan, m: Model): Draft[] {
  return plan.lines.map((l) => ({ key: nextKey++, id: l.id, name: l.name ?? m.rows.find((r) => r.id === l.id)?.name ?? "", account: l.account ?? "", share: fmtShare(l.share) }));
}

/** The model as things stand: each asset held at its share, bought where most of it is. */
function fromHoldings(m: Model): Draft[] {
  const list = m.out.filter((o) => o.value / m.invested >= 0.005);
  const shares = list.map((o) => Math.round((o.value / m.invested) * 1000) / 10);
  const diff = Math.round((100 - shares.reduce((s, x) => s + x, 0)) * 10) / 10;
  if (shares.length) shares[0] = Math.round((shares[0] + diff) * 10) / 10;
  return list.map((o, i) => ({ key: nextKey++, id: o.id, name: o.name, account: biggest(o.held), share: fmtShare(shares[i]) }));
}

function Editor({ d, start, onDone }: { d: Data; start: Draft[]; onDone: (saved: boolean) => void }) {
  const used = useLoad("assets", (s) => get<AssetRef[]>("assets", s));
  const [lines, setLines] = useState(start);
  const [sell, setSell] = useState(() => new Set(d.plan.sell));
  const [band, setBand] = useState(String(d.plan.band));
  const [cap, setCap] = useState(String(d.plan.cap));
  const [adding, setAdding] = useState<Draft>(blank);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const known = useMemo(() => new Map((used.data ?? []).map((a) => [a.id, a])), [used.data]);
  const kindOf = (x: string): PlanLine["kind"] => (known.get(x)?.kind === "security" || (!known.has(x) && ISIN.test(x)) ? "security" : "crypto");
  const total = lines.reduce((s, l) => s + (parseShare(l.share) || 0), 0);
  const crypto = lines.filter((l) => kindOf(l.id) === "crypto").reduce((s, l) => s + (parseShare(l.share) || 0), 0);
  const ok = Math.abs(total - 100) < 0.05;
  const inModel = new Set(lines.map((l) => l.id));
  const outside = d.pf.assets.filter((h) => (h.class === "stock" || h.class === "etf" || h.class === "crypto") && !inModel.has(h.id) && num(h.value) >= 1);
  const nameOf = (l: Draft) => label({ kind: kindOf(l.id), symbol: l.id, name: known.get(l.id)?.name || l.name || l.id });
  const set = (key: number, patch: Partial<Draft>) => setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)));

  const add = () => {
    if (!adding.id) return setError("Choisis un actif");
    if (inModel.has(adding.id)) return setError("Déjà dans le modèle");
    setError("");
    setLines((ls) => [...ls, { ...adding, share: adding.share || fmtShare(Math.max(0, 100 - total)) }]);
    setAdding(blank());
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const out: PlanLine[] = [];
    for (const l of lines) {
      const share = parseShare(l.share);
      if (!(share > 0)) return setError(`Une part pour ${nameOf(l)}`);
      const kind = kindOf(l.id);
      out.push({
        id: l.id,
        kind,
        share: Math.round(share * 10) / 10,
        ...(l.account ? { account: l.account } : {}),
        ...(kind === "security" && !known.get(l.id)?.name && l.name.trim() ? { name: l.name.trim() } : {}),
      });
    }
    setBusy(true);
    setError("");
    try {
      await send<Plan>("PUT", "plan", {
        ...d.plan,
        lines: out,
        sell: [...sell].filter((x) => !inModel.has(x)),
        band: Math.round(parseShare(band)),
        cap: Math.round(parseShare(cap)),
      });
      onDone(true);
    } catch (err) {
      setError(message(err));
      setBusy(false);
    }
  };

  const accountSelect = (value: string, onChange: (v: string) => void, name: string) => (
    <select className="model-acc" aria-label={`Acheté sur, ${name}`} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">Peu importe</option>
      {d.accounts.map((a) => (
        <option key={a.id} value={a.id}>
          {a.name}
        </option>
      ))}
    </select>
  );
  const shareInput = (value: string, onChange: (v: string) => void, name: string) => (
    <span className="with-unit model-share">
      <input type="text" inputMode="decimal" autoComplete="off" placeholder="0" aria-label={`Part, ${name}`} value={value} onChange={(e) => onChange(e.target.value)} />
      <span className="unit">%</span>
    </span>
  );

  return (
    <form className="model-edit" onSubmit={submit} noValidate aria-busy={busy}>
      <section className="strat-sec" aria-labelledby="h-edit">
        <h2 id="h-edit">Mon modèle</h2>
        {!used.data ? (
          <Loading small />
        ) : (
          <div className="edit-rows">
            <div className="edit-row edit-head" aria-hidden>
              <span>Actif</span>
              <span>Acheté sur</span>
              <span>Part</span>
              <span />
            </div>
            {lines.map((l) => {
              const ref = known.get(l.id);
              const name = nameOf(l);
              const unnamed = !ref && kindOf(l.id) === "security";
              return (
                <div key={l.key} className="edit-row">
                  <span className="model-name">
                    <Coin asset={{ kind: kindOf(l.id), symbol: l.id, name: ref?.name || l.name }} cls={ref?.class} size={22} />
                    {unnamed ? (
                      <input className="edit-name" type="text" maxLength={120} placeholder={`Nom de ${l.id}`} value={l.name} onChange={(e) => set(l.key, { name: e.target.value })} />
                    ) : (
                      <span className="clip">{name}</span>
                    )}
                  </span>
                  {accountSelect(l.account, (account) => set(l.key, { account }), name)}
                  {shareInput(l.share, (share) => set(l.key, { share }), name)}
                  <button type="button" className="icon-btn" aria-label={`Retirer ${name}`} onClick={() => setLines((ls) => ls.filter((x) => x.key !== l.key))}>
                    <svg viewBox="0 0 16 16" aria-hidden>
                      <path d="M4 4l8 8M12 4l-8 8" />
                    </svg>
                  </button>
                </div>
              );
            })}
            <div className="edit-row edit-add">
              <AssetPicker key={adding.key} label="Ajouter un actif" value={adding.id} used={used.data} onChange={(id) => setAdding((a) => ({ ...a, id }))} />
              {accountSelect(adding.account, (account) => setAdding((a) => ({ ...a, account })), "nouvel actif")}
              {shareInput(adding.share, (share) => setAdding((a) => ({ ...a, share })), "nouvel actif")}
              <button type="button" className="btn" onClick={add}>
                Ajouter
              </button>
            </div>
            <p className={ok ? "edit-total num muted" : "edit-total num form-error"}>
              Total {pct(total / 100, { rate: true })} · bourse {pct((total - crypto) / 100, { rate: true })}, crypto {pct(crypto / 100, { rate: true })}
              {!ok && " : il faut 100 %"}
            </p>
          </div>
        )}
      </section>

      {outside.length > 0 && (
        <section className="strat-sec" aria-labelledby="h-out">
          <h2 id="h-out">Hors modèle</h2>
          <p className="muted strat-lede">Ils comptent dans ta répartition mais ne reçoivent plus d'argent.</p>
          <ul className="out-list">
            {outside.map((h) => (
              <li key={h.id}>
                <Coin asset={h.asset} cls={h.class} size={22} />
                <span className="clip">{label(h.asset)}</span>
                <span className="num muted">{eur(h.value)}</span>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={sell.has(h.id)}
                    onChange={(e) =>
                      setSell((s) => {
                        const x = new Set(s);
                        if (e.target.checked) x.add(h.id);
                        else x.delete(h.id);
                        return x;
                      })
                    }
                  />
                  À vendre
                </label>
                <button
                  type="button"
                  className="link-btn"
                  onClick={() => setLines((ls) => [...ls, { key: nextKey++, id: h.id, name: label(h.asset), account: biggest(h), share: "" }])}
                >
                  Au modèle
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}

      <p className="edit-rules num">
        <label>
          Alerte si la crypto s'écarte de plus de
          <input type="text" inputMode="numeric" value={band} onChange={(e) => setBand(e.target.value)} />
          points,
        </label>{" "}
        <label>
          ou si une ligne hors modèle dépasse
          <input type="text" inputMode="numeric" value={cap} onChange={(e) => setCap(e.target.value)} />%
        </label>
      </p>

      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div className="actions edit-actions">
        <button type="submit" className="btn primary" disabled={busy || !ok}>
          Enregistrer
        </button>
        <button type="button" className="btn" onClick={() => onDone(false)}>
          Annuler
        </button>
      </div>
    </form>
  );
}

// ---- next contribution

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
