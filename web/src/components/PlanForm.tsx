// The strategy's settings: the model portfolio (each asset, where it is
// bought, its share), what held outside it is to be sold, the alert rules,
// the monthly plan and goal, the PEA's opening, which accounts are yours.

import { useId, useMemo, useState } from "react";
import { get, message, send, useLoad, type Account, type AssetRef, type Plan, type PlanLine, type Portfolio } from "../api";
import { ISIN, label } from "../assets";
import { pct } from "../format";
import { custody } from "../lib/strategy";
import { Dialog } from "./Dialog";
import { Loading } from "./State";
import { AssetPicker } from "./Pickers";

interface Draft {
  key: number;
  id: string;
  name: string;
  account: string;
  share: string;
}

type Num = "band" | "cap" | "monthly" | "goal" | "goalYear";

const parse = (s: string) => {
  const x = Number(s.replace(/[\s%€]/g, "").replace(",", "."));
  return s.trim() && Number.isFinite(x) ? x : NaN;
};

let next = 0;

export function PlanForm({
  plan,
  portfolio,
  accounts,
  onClose,
  onSaved,
}: {
  plan: Plan;
  portfolio: Portfolio;
  accounts: Account[];
  onClose: () => void;
  onSaved: (p: Plan) => void;
}) {
  const id = useId();
  const used = useLoad("assets", (s) => get<AssetRef[]>("assets", s));
  const [lines, setLines] = useState<Draft[]>(() =>
    plan.lines.map((l) => ({ key: next++, id: l.id, name: l.name ?? "", account: l.account ?? "", share: String(l.share).replace(".", ",") })),
  );
  const [sell, setSell] = useState(() => new Set(plan.sell));
  const [v, setV] = useState<Record<Num, string>>(() => ({
    band: String(plan.band),
    cap: String(plan.cap),
    monthly: plan.monthly ? String(plan.monthly) : "",
    goal: plan.goal ? String(plan.goal) : "",
    goalYear: plan.goalYear ? String(plan.goalYear) : "",
  }));
  const [pea, setPea] = useState(plan.pea ?? "");
  const wallets = useMemo(() => custody(portfolio, plan).list, [portfolio, plan]);
  const [own, setOwn] = useState(() => new Set(wallets.filter((w) => w.own).map((w) => w.id)));
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const known = useMemo(() => new Map((used.data ?? []).map((a) => [a.id, a])), [used.data]);
  const kindOf = (x: string): PlanLine["kind"] => (known.get(x)?.kind === "security" || (!known.has(x) && ISIN.test(x)) ? "security" : "crypto");
  const isCrypto = (x: string) => x !== "" && kindOf(x) === "crypto";

  const total = lines.reduce((s, l) => s + (parse(l.share) || 0), 0);
  const crypto = lines.filter((l) => isCrypto(l.id)).reduce((s, l) => s + (parse(l.share) || 0), 0);
  const inModel = new Set(lines.map((l) => l.id));
  const outside = portfolio.assets.filter((h) => (h.class === "stock" || h.class === "etf" || h.class === "crypto") && !inModel.has(h.id) && Number(h.value) >= 1);

  const set = (key: number, patch: Partial<Draft>) => setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)));
  const n = (k: Num) => (v[k] ? Math.round(parse(v[k])) : 0);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const out: PlanLine[] = [];
    for (const l of lines) {
      if (!l.id && !l.share) continue;
      const share = parse(l.share);
      if (!l.id) return setError("Choisis l'actif de chaque ligne");
      if (!(share > 0)) return setError("Chaque ligne a besoin d'une part");
      const kind = kindOf(l.id);
      out.push({
        id: l.id,
        kind,
        share: Math.round(share * 10) / 10,
        ...(l.account ? { account: l.account } : {}),
        ...(kind === "security" && !known.get(l.id)?.name && l.name.trim() ? { name: l.name.trim() } : {}),
      });
    }
    const body: Plan = {
      set: true,
      lines: out,
      sell: [...sell].filter((x) => !out.some((l) => l.id === x)),
      band: n("band"),
      cap: n("cap"),
      monthly: n("monthly"),
      goal: n("goal"),
      goalYear: n("goalYear"),
      pea: pea || undefined,
      custody: [...own],
    };
    if ([body.band, body.cap, body.monthly, body.goal, body.goalYear].some(Number.isNaN)) return setError("Un nombre est illisible");
    setBusy(true);
    setError("");
    try {
      onSaved(await send<Plan>("PUT", "plan", body));
    } catch (err) {
      setError(message(err));
      setBusy(false);
    }
  };

  const field = (k: Num, text: string, unit: string, hint?: string, placeholder = "0") => (
    <div className="field">
      <label htmlFor={id + k}>{text}</label>
      <span className="with-unit">
        <input
          id={id + k}
          type="text"
          inputMode="numeric"
          autoComplete="off"
          placeholder={placeholder}
          value={v[k]}
          onChange={(e) => setV((s) => ({ ...s, [k]: e.target.value }))}
        />
        {unit && <span className="unit">{unit}</span>}
      </span>
      {hint && <small className="muted">{hint}</small>}
    </div>
  );

  const ok = Math.abs(total - 100) < 0.05;
  return (
    <Dialog title="Mon modèle" onClose={onClose} wide>
      <form className="plan-form" onSubmit={submit} noValidate aria-busy={busy}>
        <fieldset>
          <legend>Portefeuille modèle</legend>
          <div className="model-lines">
            {!used.data && lines.length > 0 && <Loading small />}
            {used.data && lines.map((l) => (
              <div key={l.key} className="model-line">
                <AssetPicker label="Actif" value={l.id} used={used.data ?? []} onChange={(x) => set(l.key, { id: x })} newName={l.name} />
                <label className="field">
                  <span>Acheté sur</span>
                  <select value={l.account} onChange={(e) => set(l.key, { account: e.target.value })}>
                    <option value="">Peu importe</option>
                    {accounts.map((a) => (
                      <option key={a.id} value={a.id}>
                        {a.name}
                      </option>
                    ))}
                  </select>
                </label>
                <div className="field share">
                  <label htmlFor={id + "s" + l.key}>Part</label>
                  <span className="with-unit">
                    <input
                      id={id + "s" + l.key}
                      type="text"
                      inputMode="decimal"
                      autoComplete="off"
                      placeholder="0"
                      value={l.share}
                      onChange={(e) => set(l.key, { share: e.target.value })}
                    />
                    <span className="unit">%</span>
                  </span>
                </div>
                <button type="button" className="icon-btn" aria-label="Retirer cette ligne" onClick={() => setLines((ls) => ls.filter((x) => x.key !== l.key))}>
                  <svg viewBox="0 0 16 16" aria-hidden>
                    <path d="M4 4l8 8M12 4l-8 8" />
                  </svg>
                </button>
                {l.id && kindOf(l.id) === "security" && !known.get(l.id)?.name && (
                  <label className="field model-line-name">
                    <span>Nom</span>
                    <input type="text" maxLength={120} placeholder="iShares MSCI World Swap PEA" value={l.name} onChange={(e) => set(l.key, { name: e.target.value })} />
                  </label>
                )}
              </div>
            ))}
          </div>
          <div className="model-foot">
            <button type="button" className="btn" onClick={() => setLines((ls) => [...ls, { key: next++, id: "", name: "", account: "", share: "" }])}>
              Ajouter une ligne
            </button>
            <p className={ok || !lines.length ? "num muted" : "num form-error"}>
              Total {pct(total / 100, { rate: true })}
              {lines.length > 0 && ` · bourse ${pct((total - crypto) / 100, { rate: true })}, crypto ${pct(crypto / 100, { rate: true })}`}
            </p>
          </div>
        </fieldset>

        {outside.length > 0 && (
          <fieldset>
            <legend>Hors modèle</legend>
            <p className="muted strat-note">Ils ne reçoivent plus d'argent. Coche ceux à vendre.</p>
            <div className="checks">
              {outside.map((h) => (
                <label key={h.id} className="check">
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
                  {label(h.asset)}
                </label>
              ))}
            </div>
          </fieldset>
        )}

        <fieldset>
          <legend>Règles</legend>
          <div className="grid2">
            {field("band", "Marge de la crypto", "pts", `Alerte sous ${pct(Math.max(0, crypto - n("band")) / 100)} ou au-delà de ${pct((crypto + n("band")) / 100)}`)}
            {field("cap", "Plus grosse ligne hors modèle", "%")}
          </div>
        </fieldset>

        <fieldset>
          <legend>Plan et objectif</legend>
          <div className="grid2">
            {field("monthly", "Apport mensuel", "€", undefined, "Aucun")}
            <div className="grid2 tight">
              {field("goal", "Objectif", "€", undefined, "Aucun")}
              {field("goalYear", "En", "", undefined, String(new Date().getFullYear() + 10))}
            </div>
            <label className="field">
              <span>Ouverture du PEA</span>
              <input type="date" value={pea} onChange={(e) => setPea(e.target.value)} />
            </label>
          </div>
        </fieldset>

        {wallets.length > 0 && (
          <fieldset>
            <legend>Mes portefeuilles</legend>
            <div className="checks">
              {wallets.map((w) => (
                <label key={w.id} className="check">
                  <input
                    type="checkbox"
                    checked={own.has(w.id)}
                    onChange={(e) =>
                      setOwn((s) => {
                        const x = new Set(s);
                        if (e.target.checked) x.add(w.id);
                        else x.delete(w.id);
                        return x;
                      })
                    }
                  />
                  {w.name}
                </label>
              ))}
            </div>
          </fieldset>
        )}

        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        <div className="actions">
          <button type="submit" className="btn primary" disabled={busy || (lines.length > 0 && !ok)}>
            Enregistrer
          </button>
          <button type="button" className="btn" onClick={onClose}>
            Annuler
          </button>
        </div>
      </form>
    </Dialog>
  );
}
