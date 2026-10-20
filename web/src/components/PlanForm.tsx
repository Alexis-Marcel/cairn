// The strategy's settings: the shares aimed at, the alert rules, the
// monthly plan and goal, the PEA's opening, which accounts are your wallets.

import { useId, useState } from "react";
import { message, send, type Plan } from "../api";
import { Dialog } from "./Dialog";

type Num = "crypto" | "btc" | "eth" | "etf" | "band" | "line" | "monthly" | "goal" | "goalYear";

export function PlanForm({
  plan,
  wallets,
  onClose,
  onSaved,
}: {
  plan: Plan;
  /** Accounts holding crypto, and whether each is counted as your own wallet now. */
  wallets: { id: string; name: string; own: boolean }[];
  onClose: () => void;
  onSaved: (p: Plan) => void;
}) {
  const id = useId();
  const [v, setV] = useState<Record<Num, string>>(() => ({
    crypto: String(plan.crypto),
    btc: String(plan.btc),
    eth: String(plan.eth),
    etf: String(plan.etf),
    band: String(plan.band),
    line: String(plan.line),
    monthly: plan.monthly ? String(plan.monthly) : "",
    goal: plan.goal ? String(plan.goal) : "",
    goalYear: plan.goalYear ? String(plan.goalYear) : "",
  }));
  const [pea, setPea] = useState(plan.pea ?? "");
  const [own, setOwn] = useState(() => new Set(wallets.filter((w) => w.own).map((w) => w.id)));
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const n = (k: Num) => {
    const x = Number(v[k].replace(/\s/g, "").replace(",", "."));
    return Number.isFinite(x) ? Math.round(x) : NaN;
  };
  const rest = (a: number, b = 0) => (Number.isFinite(a) && Number.isFinite(b) ? Math.max(0, 100 - a - b) : 0);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const body: Plan = {
      set: true,
      crypto: n("crypto"),
      btc: n("btc"),
      eth: n("eth"),
      etf: n("etf"),
      band: n("band"),
      line: n("line"),
      monthly: v.monthly ? n("monthly") : 0,
      goal: v.goal ? n("goal") : 0,
      goalYear: v.goalYear ? n("goalYear") : 0,
      pea: pea || undefined,
      custody: [...own],
    };
    if (Object.values(body).some((x) => typeof x === "number" && Number.isNaN(x))) {
      setError("Un nombre est illisible");
      return;
    }
    setBusy(true);
    setError("");
    try {
      onSaved(await send<Plan>("PUT", "plan", body));
    } catch (err) {
      setError(message(err));
      setBusy(false);
    }
  };

  const field = (k: Num, label: string, unit: string, hint?: string, placeholder = "0") => (
    <div className="field">
      <label htmlFor={id + k}>{label}</label>
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
        <span className="unit">{unit}</span>
      </span>
      {hint !== undefined && <small className="muted">{hint || " "}</small>}
    </div>
  );

  return (
    <Dialog title="Ma stratégie" onClose={onClose}>
      <form className="plan-form" onSubmit={submit} noValidate aria-busy={busy}>
        <fieldset>
          <legend>Répartition cible</legend>
          <div className="grid2">
            {field("crypto", "Crypto", "%", `Bourse ${rest(n("crypto"))} %`)}
            {field("etf", "ETF dans la bourse", "%", `Actions ${rest(n("etf"))} %`)}
            {field("btc", "Bitcoin dans la crypto", "%", "")}
            {field("eth", "Ether dans la crypto", "%", `Autres cryptos ${rest(n("btc"), n("eth"))} %`)}
          </div>
        </fieldset>

        <fieldset>
          <legend>Alertes</legend>
          <div className="grid2">
            {field("band", "Écart toléré", "pts", "Crypto hors de la cible ± cet écart")}
            {field("line", "Plus grosse ligne", "%", "Pour une action ou une petite crypto")}
          </div>
        </fieldset>

        <fieldset>
          <legend>Plan et objectif</legend>
          <div className="grid2">
            {field("monthly", "Apport mensuel", "€", "", "Aucun")}
            <div className="grid2 tight">
              {field("goal", "Objectif", "€", "", "Aucun")}
              {field("goalYear", "En", "", "", String(new Date().getFullYear() + 15))}
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
                        const next = new Set(s);
                        if (e.target.checked) next.add(w.id);
                        else next.delete(w.id);
                        return next;
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
          <button type="submit" className="btn primary" disabled={busy}>
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
