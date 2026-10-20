// Operations in words: their names, and which way their coins move.

import type { Kind, Op, Origin } from "./api";
import { qty } from "./format";

export const KIND: Record<Kind, string> = {
  buy: "Achat",
  sell: "Vente",
  swap: "Échange",
  deposit: "Dépôt",
  withdrawal: "Retrait",
  reward: "Récompense",
  transfer: "Transfert",
  fee: "Frais",
  correction: "Correction",
};

/** The kinds the form offers, in its order. */
export const FORM_KINDS: Exclude<Kind, "correction">[] = ["buy", "sell", "swap", "reward", "transfer", "deposit", "withdrawal", "fee"];

export const ORIGIN: Record<Origin, string> = { manual: "Manuel", import: "Import", fundarr: "Fundarr" };

const MINUS = "−";

/** A quantity and its unit: 0,01 BTC, 60 €. */
export const amount = (q: string, symbol: string) => (symbol === "EUR" ? `${qty(q)} €` : `${qty(q)} ${symbol}`);

/** What a swap gave and got, whichever way the platform wrote it. */
export function swapLegs(op: Op) {
  return op.side === "buy" && op.quote ? { given: op.quote, got: op.base } : { given: op.base, got: op.quote! };
}

/** The coin an operation is about, its quantity signed, and for a swap what came back. */
export function moves(op: Op): { symbol: string; text: string; other?: string } {
  const out = (sign: number, q: string, symbol: string) => ({ symbol, text: `${sign > 0 ? "+" : MINUS}${amount(q, symbol)}` });
  const fiatBase = op.base.asset.kind === "fiat" && op.quote;
  switch (op.kind) {
    case "buy":
      return fiatBase ? out(1, op.quote!.qty, op.quote!.asset.symbol) : out(1, op.base.qty, op.base.asset.symbol);
    case "sell":
      return fiatBase ? out(-1, op.quote!.qty, op.quote!.asset.symbol) : out(-1, op.base.qty, op.base.asset.symbol);
    case "swap": {
      const { given, got } = swapLegs(op);
      return { ...out(-1, given.qty, given.asset.symbol), other: `+${amount(got.qty, got.asset.symbol)}` };
    }
    case "deposit":
    case "reward":
      return out(1, op.base.qty, op.base.asset.symbol);
    case "withdrawal":
    case "fee":
      return out(-1, op.base.qty, op.base.asset.symbol);
    case "transfer":
      return out(op.type === "deposit" ? 1 : -1, op.base.qty, op.base.asset.symbol);
    default: {
      const sign = op.side === "out" ? -1 : 1;
      return out(sign, op.base.qty, op.base.asset.symbol);
    }
  }
}

/** One line saying where a transfer went, or the other leg of a swap. */
export function detail(op: Op): string {
  if (op.kind === "transfer" && op.peer)
    return op.type === "deposit" ? `${op.peer.account.name} → ${op.account.name}` : `${op.account.name} → ${op.peer.account.name}`;
  return op.account.name;
}
