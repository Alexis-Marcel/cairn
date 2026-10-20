// The Stratégie page's arithmetic: where the money sits against the plan,
// where the next euros should go, what could happen, and what to watch.
// Everything comes from the portfolio; nothing here is sent anywhere.

import type { Plan, Portfolio } from "../api";
import { label } from "../assets";
import { num } from "../format";

/** The sleeves a plan aims at. Cash and stablecoins wait apart: they are what is left to invest. */
export type Sleeve = "etf" | "stock" | "btc" | "eth" | "alt";
export const SLEEVES: Sleeve[] = ["etf", "stock", "btc", "eth", "alt"];
export const SLEEVE_NAME: Record<Sleeve, string> = {
  etf: "ETF",
  stock: "Actions",
  btc: "Bitcoin",
  eth: "Ether",
  alt: "Autres cryptos",
};
export const isCrypto = (s: Sleeve) => s === "btc" || s === "eth" || s === "alt";

export type Held = Portfolio["assets"][number];

export function sleeveOf(h: Held): Sleeve | null {
  switch (h.class) {
    case "etf":
      return "etf";
    case "stock":
      return "stock";
    case "crypto":
      return h.asset.symbol === "BTC" ? "btc" : h.asset.symbol === "ETH" ? "eth" : "alt";
  }
  return null; // cash, stablecoins
}

export interface Position {
  /** Euros in each sleeve. */
  value: Record<Sleeve, number>;
  /** Invested: the sleeves' sum. Waiting: cash and stablecoins. */
  invested: number;
  waiting: number;
  crypto: number;
  bourse: number;
}

export function position(p: Portfolio): Position {
  const value = { etf: 0, stock: 0, btc: 0, eth: 0, alt: 0 };
  let waiting = num(p.total.cash);
  for (const h of p.assets) {
    const s = sleeveOf(h);
    if (s) value[s] += num(h.value);
    else waiting += num(h.value);
  }
  const crypto = value.btc + value.eth + value.alt;
  const bourse = value.etf + value.stock;
  return { value, invested: crypto + bourse, waiting, crypto, bourse };
}

/** Each sleeve's share of the invested whole the plan aims at. */
export function targets(plan: Plan): Record<Sleeve, number> {
  const c = plan.crypto / 100;
  const b = plan.btc / 100;
  const e = plan.eth / 100;
  const f = plan.etf / 100;
  return { btc: c * b, eth: c * e, alt: c * Math.max(0, 1 - b - e), etf: (1 - c) * f, stock: (1 - c) * (1 - f) };
}

/**
 * Where amount should go to come closer to the plan without selling: first
 * to what is short of its share, in proportion, then along the plan.
 * Whole euros, adding up to the amount.
 */
export function nextSplit(pos: Position, plan: Plan, amount: number): Record<Sleeve, number> {
  const t = targets(plan);
  const total = pos.invested + amount;
  const gap = Object.fromEntries(SLEEVES.map((s) => [s, Math.max(0, t[s] * total - pos.value[s])])) as Record<Sleeve, number>;
  const short = SLEEVES.reduce((sum, s) => sum + gap[s], 0);
  const raw = Object.fromEntries(
    SLEEVES.map((s) => [s, short >= amount ? (short > 0 ? (amount * gap[s]) / short : 0) : gap[s] + (amount - short) * t[s]]),
  ) as Record<Sleeve, number>;
  // Round to euros, the remainder going to the largest parts.
  const out = Object.fromEntries(SLEEVES.map((s) => [s, Math.floor(raw[s])])) as Record<Sleeve, number>;
  let left = Math.round(amount) - SLEEVES.reduce((sum, s) => sum + out[s], 0);
  for (const s of [...SLEEVES].sort((a, b) => raw[b] - Math.floor(raw[b]) - (raw[a] - Math.floor(raw[a])))) {
    if (left <= 0) break;
    out[s]++;
    left--;
  }
  return out;
}

/** The value month by month, growing at rate a year with monthly put in each month. */
export function grow(start: number, monthly: number, rate: number, months: number): number[] {
  const r = Math.pow(1 + rate, 1 / 12) - 1;
  const out = [start];
  let v = start;
  for (let m = 0; m < months; m++) {
    v = v * (1 + r) + monthly;
    out.push(v);
  }
  return out;
}

/** The yearly returns the projection draws: prudent, middle, favourable. */
export const RATES = [
  { key: "low", label: "Prudent", rate: 0.02 },
  { key: "mid", label: "Moyen", rate: 0.05 },
  { key: "high", label: "Favorable", rate: 0.08 },
] as const;

/** The first month index at which a series reaches goal, or -1. */
export const reached = (series: number[], goal: number) => series.findIndex((v) => v >= goal);

/** Crises to test: what crypto and stocks lose. */
export const CRISES = [
  { key: "crypto", label: "Krach crypto", crypto: -0.7, bourse: -0.1 },
  { key: "bourse", label: "Krach boursier", crypto: -0.5, bourse: -0.4 },
  { key: "both", label: "Les deux au pire", crypto: -0.8, bourse: -0.5 },
] as const;

export function crisisLoss(pos: Position, c: (typeof CRISES)[number]): number {
  return pos.crypto * c.crypto + pos.bourse * c.bourse;
}

/** The lines that weigh, of the invested whole: an ETF counts as one line. */
export function concentration(p: Portfolio, invested: number) {
  const lines = p.assets
    .filter((h) => sleeveOf(h))
    .map((h) => ({ id: h.id, name: label(h.asset), value: num(h.value), sleeve: sleeveOf(h)!, asset: h.asset, cls: h.class }))
    .sort((a, b) => b.value - a.value);
  const share = (v: number) => (invested > 0 ? v / invested : 0);
  return {
    lines,
    top: lines[0] ? { ...lines[0], share: share(lines[0].value) } : null,
    top5: share(lines.slice(0, 5).reduce((s, l) => s + l.value, 0)),
    small: lines.filter((l) => share(l.value) < 0.01).length,
    share,
  };
}

// Names of self-custody wallets, for a first guess of which accounts are yours.
const WALLET = /ledger|trezor|wallet|portefeuille|metamask|phantom|exodus|trust|bitbox|coldcard|electrum|sparrow|tangem|keystone/i;

/** Accounts holding crypto, with what they hold, and whether they are the owner's own wallets. */
export function custody(p: Portfolio, plan: Plan) {
  const by = new Map<string, { id: string; name: string; value: number; btc: number }>();
  for (const h of p.assets) {
    if (h.class !== "crypto" && h.class !== "stable") continue;
    const price = num(h.price);
    for (const s of h.sources) {
      const a = by.get(s.id) ?? { id: s.id, name: s.name, value: 0, btc: 0 };
      const v = num(s.qty) * price;
      a.value += v;
      if (h.asset.symbol === "BTC") a.btc += v;
      by.set(s.id, a);
    }
  }
  const own = (id: string, name: string) => (plan.custody ? plan.custody.includes(id) : WALLET.test(name));
  const list = [...by.values()].filter((a) => a.value >= 1).map((a) => ({ ...a, own: own(a.id, a.name) })).sort((a, b) => b.value - a.value);
  const total = list.reduce((s, a) => s + a.value, 0);
  const mine = list.filter((a) => a.own).reduce((s, a) => s + a.value, 0);
  const btc = list.reduce((s, a) => s + a.btc, 0);
  const btcMine = list.filter((a) => a.own).reduce((s, a) => s + a.btc, 0);
  return { list, total, mine, btc, btcMine };
}

export const isPEA = (name: string) => /\bPEA\b/i.test(name);

/** Each stock or ETF held outside a PEA: what selling it would cost in tax. */
export function ctoLines(p: Portfolio, rate: number) {
  const out: { id: string; name: string; asset: Held["asset"]; cls: Held["class"]; value: number; gain: number; tax: number }[] = [];
  for (const h of p.assets) {
    if (h.class !== "stock" && h.class !== "etf") continue;
    const q = num(h.qty);
    const cto = h.sources.filter((s) => !isPEA(s.name)).reduce((s, x) => s + num(x.qty), 0);
    if (q <= 0 || cto <= 0) continue;
    const part = cto / q;
    const gain = num(h.unrealized) * part;
    out.push({ id: h.id, name: label(h.asset), asset: h.asset, cls: h.class, value: num(h.value) * part, gain, tax: Math.max(0, gain) * rate });
  }
  return out.sort((a, b) => b.value - a.value);
}

// Platforms held abroad: their accounts are declared each year (3916, 3916-bis for crypto).
const ABROAD: [RegExp, string, boolean][] = [
  [/binance/i, "Binance", true],
  [/okx/i, "OKX", true],
  [/kraken/i, "Kraken", true],
  [/coinbase/i, "Coinbase", true],
  [/bitpanda/i, "Bitpanda", true],
  [/bitstamp/i, "Bitstamp", true],
  [/crypto\.com/i, "Crypto.com", true],
  [/bybit/i, "Bybit", true],
  [/kucoin/i, "KuCoin", true],
  [/trade ?republic/i, "Trade Republic", false],
  [/scalable/i, "Scalable Capital", false],
  [/degiro/i, "Degiro", false],
  [/interactive ?brokers|\bibkr\b/i, "Interactive Brokers", false],
  [/etoro/i, "eToro", false],
  [/revolut/i, "Revolut", false],
];

/** The platforms abroad among the accounts, each once. */
export function abroad(names: string[]) {
  const out = new Map<string, boolean>();
  for (const n of names) for (const [re, name, crypto] of ABROAD) if (re.test(n)) out.set(name, crypto);
  return [...out].map(([name, crypto]) => ({ name, form: crypto ? "3916-bis" : "3916" }));
}

/** The day the PEA turns five, and the days left. */
export function peaFive(opened: string, now = new Date()) {
  const d = new Date(opened + "T00:00:00Z");
  const five = new Date(Date.UTC(d.getUTCFullYear() + 5, d.getUTCMonth(), d.getUTCDate()));
  const days = Math.ceil((five.getTime() - now.getTime()) / 86400000);
  return { five, days };
}
