// Adding or changing an operation: pick what happened, then only the fields
// it needs. The price of a purchase or a sale is filled in from the day's
// close until it is typed over.

import { useEffect, useId, useMemo, useState } from "react";
import { get, message, send, type Account, type AssetRef, type Entry, type Kind, type Op } from "../api";
import { ISIN, assetId, label, validISIN } from "../assets";
import { date as longDate, eur, num } from "../format";
import { ADJUST_WHY, FORM_KINDS, KIND, ORIGIN, amount, detail, moves, noteOf, swapLegs, title } from "../ops";
import { Coin } from "./Coin";
import { AccountPicker, AssetPicker, type AccountChoice } from "./Pickers";

type FormKind = Exclude<Kind, "correction">;

const today = () => {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};

/** A decimal string written the French way: 0,5. */
const fr = (s: string | undefined) => (s ?? "").replace(".", ",");

/** A decimal as typed ("1 234,5") as a number. */
const parse = (s: string) => Number(s.replace(/[\s  €]/g, "").replace(",", "."));

interface State {
  kind: FormKind;
  date: string;
  account: AccountChoice;
  to: AccountChoice;
  asset: string;
  name: string;
  qty: string;
  total: string;
  fee: string;
  received: string;
  receivedQty: string;
  note: string;
}

/** The form's fields for an operation already there. */
function fromOp(op: Op): State {
  const s: State = {
    kind: op.kind === "correction" ? "deposit" : op.kind,
    date: op.time.slice(0, 10),
    account: { id: op.account.id },
    to: {},
    asset: assetId(op.base.asset),
    name: op.base.asset.name ?? "",
    qty: fr(op.base.qty),
    total: "",
    fee: op.fee && op.fee.asset.symbol === "EUR" ? fr(op.fee.qty) : "",
    received: "",
    receivedQty: "",
    note: op.note ?? "",
  };
  switch (op.kind) {
    case "buy":
    case "sell":
      s.total = fr(op.quote?.qty);
      break;
    case "dividend":
      if (op.quote) Object.assign(s, { asset: assetId(op.quote.asset), name: op.quote.asset.name ?? "" });
      break;
    case "swap": {
      const { given, got } = swapLegs(op);
      Object.assign(s, { asset: given.asset.symbol, qty: fr(given.qty), received: got.asset.symbol, receivedQty: fr(got.qty) });
      break;
    }
    case "transfer":
      if (!op.linked || !op.peer) {
        s.kind = op.type === "deposit" ? "deposit" : "withdrawal";
        break;
      }
      if (op.type === "deposit") {
        s.account = { id: op.peer.account.id };
        s.to = { id: op.account.id };
        s.qty = fr(op.base.qty);
        s.fee = fr(op.peer.fee?.qty);
      } else {
        s.to = { id: op.peer.account.id };
        s.fee = fr(op.fee?.qty);
      }
      break;
  }
  return s;
}

const accountOf = (c: AccountChoice) => (c.name !== undefined ? { newAccount: c.name } : { account: c.id });

export function OpForm({
  op,
  asset,
  accounts,
  used,
  onSaved,
  onCancel,
}: {
  op?: Op;
  asset?: string;
  accounts: Account[];
  used: AssetRef[];
  onSaved: () => void;
  onCancel: () => void;
}) {
  const id = useId();
  const first = accounts.find((a) => a.origin === "manual") ?? accounts[0];
  const preset = asset ? used.find((a) => a.id === asset) : undefined;
  const [s, setS] = useState<State>(() =>
    op
      ? fromOp(op)
      : {
          kind: "buy",
          date: today(),
          account: first ? { id: first.id } : { name: "" },
          to: {},
          asset: preset?.id ?? "",
          name: preset?.name ?? "",
          qty: "",
          total: "",
          fee: "",
          received: "",
          receivedQty: "",
          note: "",
        },
  );
  const [touched, setTouched] = useState(!!op);
  const [unit, setUnit] = useState<{ key: string; price: number } | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const set = (p: Partial<State>) => setS((o) => ({ ...o, ...p }));

  const k = s.kind;
  const priced = k === "buy" || k === "sell";
  const cash = k === "deposit" || k === "withdrawal" || k === "fee" || k === "interest";
  const income = k === "dividend" || k === "interest";
  // A stock or an ETF: by its ISIN, named when Cairn does not know it yet.
  const isin = ISIN.test(s.asset);
  const known = used.find((a) => a.id === s.asset);
  const needsName = isin && !known?.name;
  const badIsin = isin && !validISIN(s.asset);

  // The day's close of the coin, for the price.
  const key = priced && s.asset && s.date ? `${s.asset}|${s.date}` : "";
  useEffect(() => {
    if (!key) return;
    const ctl = new AbortController();
    const [asset, date] = key.split("|");
    const t = setTimeout(() => {
      get<{ price: string }>(`price?asset=${encodeURIComponent(asset)}&date=${date}`, ctl.signal).then(
        (r) => setUnit({ key, price: num(r.price) }),
        () => !ctl.signal.aborted && setUnit({ key, price: 0 }),
      );
    }, 250);
    return () => {
      clearTimeout(t);
      ctl.abort();
    };
  }, [key]);
  const price = unit && unit.key === key && unit.price > 0 ? unit.price : 0;
  const q = parse(s.qty);
  const suggested = price && q > 0 ? fr((price * q).toFixed(2)) : "";
  const total = touched || !priced ? s.total : suggested;

  // Switching to a deposit starts in euros; back from it, to a coin; a
  // dividend wants a security.
  const pickKind = (kind: FormKind) => {
    const toCash = kind === "deposit" || kind === "withdrawal" || kind === "interest";
    const wasCash = k === "deposit" || k === "withdrawal" || k === "interest";
    let asset = toCash && !s.asset ? "EUR" : !toCash && wasCash && s.asset === "EUR" ? "" : s.asset;
    if (kind === "dividend" && !ISIN.test(asset)) asset = "";
    if (kind === "interest") asset = "EUR";
    set({ kind, asset });
  };

  const entry = (): Entry => {
    const e: Entry = { kind: k, date: s.date, asset: s.asset, qty: s.qty, note: s.note || undefined, ...accountOf(s.account) };
    if (isin) e.name = (needsName ? s.name : known?.name || s.name).trim() || undefined;
    if (priced) Object.assign(e, { total, fee: s.fee });
    if (k === "swap") Object.assign(e, { received: s.received, receivedQty: s.receivedQty, fee: s.fee });
    if (k === "transfer") {
      Object.assign(e, { fee: s.fee });
      if (s.to.name !== undefined) e.newTo = s.to.name;
      else e.to = s.to.id;
    }
    return e;
  };

  const submit = async (ev: React.FormEvent) => {
    ev.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await (op ? send("PUT", `operations/${op.id}`, entry()) : send("POST", "operations", entry()));
      onSaved();
    } catch (err) {
      setError(message(err));
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!op) return;
    setBusy(true);
    setError("");
    try {
      await send("DELETE", `operations/${op.id}`);
      onSaved();
    } catch (err) {
      setError(message(err));
      setBusy(false);
    }
  };

  const ready =
    !!s.date &&
    (s.account.id || s.account.name?.trim()) &&
    !!s.asset &&
    !badIsin &&
    (!needsName || !!s.name.trim()) &&
    q > 0 &&
    (!priced || parse(total) > 0) &&
    (k !== "swap" || (!!s.received && parse(s.receivedQty) > 0)) &&
    (k !== "transfer" || !!(s.to.id || s.to.name?.trim()));

  const qtyLabel = income ? "Reçu (€)" : s.asset === "EUR" ? "Montant (€)" : k === "fee" ? "Frais" : k === "swap" ? "Quantité donnée" : "Quantité";
  const kinds = FORM_KINDS.includes(k) ? FORM_KINDS : [...FORM_KINDS, k];

  return (
    <form className="op-form" onSubmit={submit} noValidate aria-busy={busy}>
      <fieldset className="kinds">
        <legend className="sr">Opération</legend>
        {kinds.map((kind) => (
          <label key={kind} className={kind === k ? "chip on" : "chip"}>
            <input type="radio" name={id + "-kind"} value={kind} checked={kind === k} onChange={() => pickKind(kind)} />
            {KIND[kind]}
          </label>
        ))}
      </fieldset>

      <div className="grid2">
        <label className="field">
          <span>Date</span>
          <input type="date" required max={today()} value={s.date} onChange={(e) => set({ date: e.target.value })} />
        </label>
        <AccountPicker
          label={k === "transfer" ? "De" : "Compte"}
          accounts={accounts}
          value={s.account}
          onChange={(account) => set({ account })}
        />
        {k === "transfer" && (
          <AccountPicker label="Vers" accounts={accounts} value={s.to} exclude={s.account.id} onChange={(to) => set({ to })} />
        )}
      </div>

      <div className="grid2">
        {k !== "interest" && (
          <AssetPicker
            key={k + (cash ? "c" : "")}
            label={k === "swap" ? "Donné" : k === "dividend" ? "Versé par" : "Actif"}
            value={s.asset}
            used={used}
            cash={cash}
            coins={k !== "dividend"}
            securities={k !== "swap"}
            newName={s.name}
            onChange={(asset) => set({ asset })}
          />
        )}
        <label className="field">
          <span>{qtyLabel}</span>
          <input
            type="text"
            inputMode="decimal"
            autoComplete="off"
            placeholder="0"
            value={s.qty}
            onChange={(e) => set({ qty: e.target.value })}
          />
        </label>
        {needsName && (
          <div className="field wide">
            <label htmlFor={id + "-name"}>Nom</label>
            <input
              id={id + "-name"}
              type="text"
              autoComplete="off"
              maxLength={80}
              placeholder="iShares Core MSCI World"
              aria-describedby={id + "-isin"}
              value={s.name}
              onChange={(e) => set({ name: e.target.value })}
            />
            <small id={id + "-isin"} className={badIsin ? "form-error" : "muted"}>
              {badIsin ? "ISIN invalide : vérifie-le" : `Nouveau titre · ${s.asset}`}
            </small>
          </div>
        )}
        {k === "swap" && (
          <>
            <AssetPicker label="Reçu" value={s.received} used={used} securities={false} onChange={(received) => set({ received })} />
            <label className="field">
              <span>Quantité reçue</span>
              <input
                type="text"
                inputMode="decimal"
                autoComplete="off"
                placeholder="0"
                value={s.receivedQty}
                onChange={(e) => set({ receivedQty: e.target.value })}
              />
            </label>
          </>
        )}
      </div>

      {(priced || k === "swap" || k === "transfer") && (
        <div className="grid2">
          {priced && (
            <div className="field">
              <label htmlFor={id + "-total"}>{k === "buy" ? "Payé (€)" : "Reçu (€)"}</label>
              <input
                id={id + "-total"}
                type="text"
                inputMode="decimal"
                autoComplete="off"
                placeholder={suggested || "0"}
                aria-describedby={id + "-hint"}
                value={total}
                onChange={(e) => {
                  setTouched(true);
                  set({ total: e.target.value });
                }}
              />
              <small id={id + "-hint"} className="muted">
                {price ? `1 ${isin ? "part" : s.asset} ≈ ${eur(price)} ce jour-là` : " "}
              </small>
            </div>
          )}
          <label className="field">
            <span>{k === "transfer" ? `Frais réseau${s.asset ? ` (${s.asset})` : ""}` : "Frais (€)"}</span>
            <input
              type="text"
              inputMode="decimal"
              autoComplete="off"
              placeholder="Aucun"
              value={s.fee}
              onChange={(e) => set({ fee: e.target.value })}
            />
          </label>
        </div>
      )}

      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}

      {confirm ? (
        <div className="actions confirm-row">
          <span>Supprimer cette opération ?</span>
          <button type="button" className="btn danger" onClick={remove} disabled={busy}>
            Supprimer
          </button>
          <button type="button" className="btn quiet" onClick={() => setConfirm(false)} disabled={busy}>
            Annuler
          </button>
        </div>
      ) : (
        <div className="actions">
          <button type="submit" className="btn primary" disabled={busy || !ready}>
            {busy ? "Enregistrement…" : op ? "Enregistrer" : "Ajouter"}
          </button>
          <button type="button" className="btn quiet" onClick={onCancel}>
            Annuler
          </button>
          {op && (
            <button type="button" className="btn quiet push danger-text" onClick={() => setConfirm(true)}>
              Supprimer
            </button>
          )}
        </div>
      )}
    </form>
  );
}

/** An operation copied from Fundarr: shown, not edited. */
export function OpDetails({ op, onClose }: { op: Op; onClose: () => void }) {
  const m = moves(op);
  const rows = useMemo(() => {
    const r: [string, string][] = [
      ["Date", longDate(op.time)],
      [op.kind === "transfer" ? "Comptes" : "Compte", detail(op)],
      ["Quantité", m.other ? `${m.text}, ${m.other}` : m.text],
    ];
    if (op.value) r.push(["Valeur", eur(op.value, { cents: true })]);
    if (op.fee) r.push(["Frais", amount(op.fee.qty, op.fee.asset)]);
    const isin = (op.kind === "dividend" ? op.quote?.asset : op.base.asset)?.isin;
    if (isin) r.push(["ISIN", isin]);
    if (op.note) r.push(["Note", noteOf(op.note)]);
    return r;
  }, [op, m]);
  return (
    <div className="op-details">
      <p className="op-title">
        <Coin asset={m.asset} />
        <span>
          {title(op)} {m.asset.kind === "security" && label(m.asset)} <span className={`badge ${op.origin}`}>{ORIGIN[op.origin]}</span>
        </span>
      </p>
      <dl>
        {rows.map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd className="num">{v}</dd>
          </div>
        ))}
      </dl>
      <p className="muted note">{op.kind === "correction" ? ADJUST_WHY : "Elle vient de Fundarr : modifie-la dans Fundarr."}</p>
      <div className="actions">
        <button type="button" className="btn" onClick={onClose}>
          Fermer
        </button>
      </div>
    </div>
  );
}
