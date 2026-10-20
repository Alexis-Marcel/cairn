// Operations in words: their names, and which way their coins move.

import type { Asset, Kind, Op, Origin } from "./api";
import { unit } from "./assets";
import { qty } from "./format";

export const KIND: Record<Kind, string> = {
  buy: "Achat",
  sell: "Vente",
  swap: "Échange",
  deposit: "Dépôt",
  withdrawal: "Retrait",
  reward: "Récompense",
  dividend: "Dividende",
  interest: "Intérêts",
  transfer: "Transfert",
  fee: "Frais",
  correction: "Ajustement",
};

/** What Fundarr's adjustments are, in a sentence. */
export const ADJUST_WHY = "Fundarr corrige un historique incomplet. Importer le relevé de la plateforme les remplace.";

// Fundarr's notes on its corrections, in French.
const NOTES: Record<string, string> = {
  "held on the platform but missing from its history": "Détenu sur la plateforme, absent de son historique",
  "in the history but no longer held on the platform": "Dans l'historique, plus détenu sur la plateforme",
  "missing from the platform's history: it shows more leaving than ever arrived": "Absent de l'historique : il en sort plus qu'il n'en est arrivé",
};

/** A note as shown: Fundarr's own in French. */
export const noteOf = (n: string) => NOTES[n] ?? n;

/**
 * What an operation is called in a list: its kind, or more exactly when the
 * kind says too little (coins sent outside, a card payment).
 */
export function title(op: Op): string {
  if (op.type === "tax") return "Impôt";
  const fiat = op.base.asset.kind === "fiat";
  const note = (op.note ?? "").toLowerCase();
  switch (op.kind) {
    case "deposit":
      if (!fiat) return "Reçu de l'extérieur";
      if (note.startsWith("card refund")) return "Remboursement carte";
      break;
    case "withdrawal":
      if (!fiat) return "Envoi externe";
      if (note.startsWith("cash withdrawal")) return "Retrait d'espèces";
      if (note.startsWith("card")) return "Paiement par carte";
      break;
    case "fee":
      if (fiat && note.startsWith("card")) return "Frais de carte";
      break;
  }
  return KIND[op.kind];
}

/** The kinds the form offers, in its order. */
export const FORM_KINDS: Exclude<Kind, "correction">[] = ["buy", "sell", "swap", "reward", "dividend", "transfer", "deposit", "withdrawal", "fee"];

export const ORIGIN: Record<Origin, string> = { manual: "Manuel", import: "Import", fundarr: "Fundarr" };

const MINUS = "−";

/** A quantity and its unit: 0,01 BTC, 60 €, 3 parts. */
export const amount = (q: string, a: Pick<Asset, "kind" | "symbol">) =>
  a.kind === "fiat" && a.symbol === "EUR" ? `${qty(q)} €` : `${qty(q)} ${unit(a)}`;

/** What a swap gave and got, whichever way the platform wrote it. */
export function swapLegs(op: Op) {
  return op.side === "buy" && op.quote ? { given: op.quote, got: op.base } : { given: op.base, got: op.quote! };
}

/**
 * The asset an operation is about, its quantity signed, and for a swap what
 * came back. A dividend is about its stock or ETF, in euros.
 */
export function moves(op: Op): { asset: Asset; text: string; other?: string } {
  const out = (sign: number, q: string, asset: Asset) => ({ asset, text: `${sign > 0 ? "+" : MINUS}${amount(q, asset)}` });
  const fiatBase = op.base.asset.kind === "fiat" && op.quote;
  switch (op.kind) {
    case "buy":
      return fiatBase ? out(1, op.quote!.qty, op.quote!.asset) : out(1, op.base.qty, op.base.asset);
    case "sell":
      return fiatBase ? out(-1, op.quote!.qty, op.quote!.asset) : out(-1, op.base.qty, op.base.asset);
    case "swap": {
      const { given, got } = swapLegs(op);
      return { ...out(-1, given.qty, given.asset), other: `+${amount(got.qty, got.asset)}` };
    }
    case "dividend":
      return { ...out(1, op.base.qty, op.base.asset), asset: op.quote?.asset ?? op.base.asset };
    case "deposit":
    case "reward":
    case "interest":
      return out(1, op.base.qty, op.base.asset);
    case "withdrawal":
    case "fee":
      return out(-1, op.base.qty, op.base.asset);
    case "transfer":
      return out(op.type === "deposit" ? 1 : -1, op.base.qty, op.base.asset);
    default: {
      const sign = op.side === "out" ? -1 : 1;
      return out(sign, op.base.qty, op.base.asset);
    }
  }
}

/** One line saying where a transfer went, or the other leg of a swap. */
export function detail(op: Op): string {
  if (op.kind === "transfer" && op.peer)
    return op.type === "deposit" ? `${op.peer.account.name} → ${op.account.name}` : `${op.account.name} → ${op.peer.account.name}`;
  return op.account.name;
}
