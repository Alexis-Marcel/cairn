// The Stratégie page's arithmetic. The plan is a model portfolio: each line
// an asset, where it is bought and its share of everything invested. What
// is held outside it ("hors plan") still counts in the crypto/bourse split
// but never receives money. From that: where the money sits against the
// model, where the next euros go, and what to do now, tax-free moves first.

import type { Account, Plan, PlanLine, Portfolio } from "../api";
import { label } from "../assets";
import { num } from "../format";

export type Held = Portfolio["assets"][number];

/** Held, and worth planning: stocks, ETFs, coins (not cash, not stablecoins). */
const planned = (h: Held) => h.class === "stock" || h.class === "etf" || h.class === "crypto";

export const isPEA = (name: string) => /\bPEA\b/i.test(name);

// Names of self-custody wallets, for a first guess of which accounts are yours.
const WALLET = /ledger|trezor|wallet|portefeuille|metamask|phantom|exodus|trust|bitbox|coldcard|electrum|sparrow|tangem|keystone/i;

export const ownWallet = (plan: Plan, id: string, name: string) => (plan.custody ? plan.custody.includes(id) : WALLET.test(name));

/** A line of the model, with what is held of it. */
export interface Row {
  line: PlanLine;
  id: string;
  name: string;
  asset: Held["asset"];
  cls: Held["class"];
  crypto: boolean;
  value: number;
  /** Share of everything invested, aimed at. */
  target: number;
  /** Where it is bought, in words. */
  where?: string;
  held?: Held;
}

/** Something held outside the model. */
export interface Out {
  id: string;
  name: string;
  asset: Held["asset"];
  cls: Held["class"];
  crypto: boolean;
  value: number;
  sell: boolean;
  held: Held;
}

export interface Model {
  rows: Row[];
  out: Out[];
  /** Everything invested (model and outside it); cash and stablecoins wait apart. */
  invested: number;
  waiting: number;
  crypto: number;
  bourse: number;
  /** The crypto share the model aims at. */
  cryptoTarget: number;
}

export function model(p: Portfolio, plan: Plan, accounts: Account[]): Model {
  const byId = new Map(p.assets.filter(planned).map((h) => [h.id, h]));
  const accName = (id?: string) => (id ? accounts.find((a) => a.id === id)?.name : undefined);
  const rows: Row[] = plan.lines.map((line) => {
    const h = byId.get(line.id);
    const asset = h?.asset ?? (line.kind === "security" ? { kind: "security" as const, symbol: line.id, isin: line.id, name: line.name } : { kind: "crypto" as const, symbol: line.id });
    const cls = h?.class ?? (line.kind === "crypto" ? "crypto" : /^(IE|LU)/.test(line.id) || /etf|msci|s&p|world|\(acc\)/i.test(line.name ?? "") ? "etf" : "stock");
    return {
      line,
      id: line.id,
      name: h ? label(h.asset) : line.kind === "security" ? line.name || line.id : line.id,
      asset,
      cls,
      crypto: line.kind === "crypto",
      value: h ? num(h.value) : 0,
      target: line.share / 100,
      where: accName(line.account),
      held: h,
    };
  });
  const inModel = new Set(plan.lines.map((l) => l.id));
  const out: Out[] = [...byId.values()]
    .filter((h) => !inModel.has(h.id) && num(h.value) >= 1)
    .map((h) => ({ id: h.id, name: label(h.asset), asset: h.asset, cls: h.class, crypto: h.class === "crypto", value: num(h.value), sell: plan.sell.includes(h.id), held: h }))
    .sort((a, b) => b.value - a.value);
  let waiting = num(p.total.cash);
  for (const h of p.assets) if (!planned(h)) waiting += num(h.value);
  const all = [...rows, ...out];
  const crypto = all.filter((x) => x.crypto).reduce((s, x) => s + x.value, 0);
  const bourse = all.filter((x) => !x.crypto).reduce((s, x) => s + x.value, 0);
  const cryptoTarget = rows.filter((r) => r.crypto).reduce((s, r) => s + r.target, 0);
  return { rows, out, invested: crypto + bourse, waiting, crypto, bourse, cryptoTarget };
}

/**
 * Where amount goes to come closer to the model without selling: first to
 * the lines short of their share, in proportion, then along the model.
 * Whole euros adding up to the amount, by line id.
 */
export function nextSplit(m: Model, amount: number): Map<string, number> {
  const total = m.invested + amount;
  const gap = m.rows.map((r) => Math.max(0, r.target * total - r.value));
  const short = gap.reduce((s, g) => s + g, 0);
  const raw = m.rows.map((r, i) => (short >= amount ? (short > 0 ? (amount * gap[i]) / short : 0) : gap[i] + (amount - short) * r.target));
  const out = raw.map(Math.floor);
  let left = Math.round(amount) - out.reduce((s, v) => s + v, 0);
  const order = raw.map((v, i) => [v - Math.floor(v), i]).sort((a, b) => b[0] - a[0]);
  for (const [, i] of order) {
    if (left <= 0) break;
    out[i]++;
    left--;
  }
  return new Map(m.rows.map((r, i) => [r.id, out[i]]));
}

/** Where most of a held asset sits: the account's name. */
function mostlyOn(h: Held | undefined): string | undefined {
  if (!h?.sources.length) return undefined;
  return [...h.sources].sort((a, b) => num(b.qty) - num(a.qty))[0].name;
}

export interface Todo {
  key: string;
  text: string;
  /** Tax-free, taxed (with an estimate), or nothing to say. */
  tax?: { free: true } | { free: false; amount: number };
  level?: "warn" | "info";
  /** The amount to place, for "Prochain apport". */
  place?: number;
}

const round = (v: number) => Math.round(v / 10) * 10;
const fmt = (v: number) => new Intl.NumberFormat("fr-FR", { maximumFractionDigits: 0 }).format(v) + " €";

/**
 * What to do now, in order: the moves that cost no tax (swaps between
 * coins, arbitrages inside a PEA), the cash to place, the sales asked for,
 * crypto out of its band, coins to take home, lines that weigh too much.
 * Each goes away by itself once done.
 */
export function todo(m: Model, plan: Plan, p: Portfolio, rate: number): Todo[] {
  const out: Todo[] = [];
  if (!plan.set || m.invested <= 0) return out;

  // Coins: swaps cost no tax. Within the crypto, each model line aims at its
  // part; coins outside the model are swapped into the lines short of theirs.
  const cRows = m.rows.filter((r) => r.crypto);
  const cTarget = cRows.reduce((s, r) => s + r.target, 0);
  if (cRows.length && m.crypto > 0) {
    const min = Math.max(50, 0.05 * m.crypto);
    const over: { name: string; amount: number; on?: string; outside: boolean }[] = [];
    const under: { name: string; amount: number }[] = [];
    for (const r of cRows) {
      const d = r.value - (r.target / cTarget) * m.crypto;
      if (d >= min) over.push({ name: r.name, amount: d, on: mostlyOn(r.held), outside: false });
      else if (-d >= 1) under.push({ name: r.name, amount: -d });
    }
    const small = m.out.filter((o) => o.crypto && o.value >= 10);
    const smallTotal = small.reduce((s, o) => s + o.value, 0);
    under.sort((a, b) => b.amount - a.amount);
    const to = under[0]?.name;
    for (const o of over) {
      if (!to) break;
      out.push({ key: "swap-" + o.name, text: `Échange ${fmt(round(o.amount))} de ${o.name} contre du ${to}${o.on ? ` sur ${o.on}` : ""}`, tax: { free: true } });
    }
    if (to && smallTotal >= 20) {
      const names = small.slice(0, 3).map((o) => o.name).join(", ") + (small.length > 3 ? ` et ${small.length - 3} autres` : "");
      const where = [...new Set(small.map((o) => mostlyOn(o.held)).filter(Boolean))].join(", ");
      out.push({ key: "swap-small", text: `Échange tes autres cryptos (${names}, ${fmt(round(smallTotal))}) contre du ${to}${where ? ` sur ${where}` : ""}`, tax: { free: true } });
    }
  }

  // Securities to sell: inside a PEA it costs no tax, the money stays there.
  const peaLines = m.rows.filter((r) => !r.crypto && r.where && isPEA(r.where));
  const short = (rows: Row[]) =>
    [...rows].sort((a, b) => b.target * m.invested - b.value - (a.target * m.invested - a.value))[0];
  for (const o of m.out.filter((x) => x.sell && !x.crypto)) {
    const inPEA = o.held.sources.filter((s) => isPEA(s.name)).reduce((s, x) => s + num(x.qty), 0);
    const q = num(o.held.qty) || 1;
    const peaValue = (o.value * inPEA) / q;
    const ctoValue = o.value - peaValue;
    if (peaValue >= 1) {
      const buy = short(peaLines);
      out.push({ key: "pea-" + o.id, text: `Dans le PEA, vends ${o.name} (${fmt(round(peaValue))})${buy ? ` et achète ${buy.name}` : ""}`, tax: { free: true } });
    }
    if (ctoValue >= 1) {
      const gain = (num(o.held.unrealized) * (q - inPEA)) / q;
      out.push({ key: "sell-" + o.id, text: `Vends ${o.name} (${fmt(round(ctoValue))})`, tax: { free: false, amount: Math.max(0, gain) * rate } });
    }
  }

  // Cash waiting.
  if (m.waiting >= 100) out.push({ key: "cash", text: `Place tes ${fmt(Math.round(m.waiting))} de liquidités`, place: Math.round(m.waiting) });

  // Crypto out of its band: above, sell (taxed); below, the next euros go there.
  const share = m.crypto / m.invested;
  const band = plan.band / 100;
  if (share > m.cryptoTarget + band) {
    const sell = (share - m.cryptoTarget) * m.invested;
    const cr = p.assets.filter((a) => a.class === "crypto" || a.class === "stable");
    const v = cr.reduce((s, a) => s + num(a.value), 0);
    const c = cr.reduce((s, a) => s + num(a.cost), 0);
    const gainPart = v > 0 ? Math.max(0, 1 - c / v) : 0;
    out.push({
      key: "crypto-high",
      text: `La crypto pèse ${Math.round(share * 100)} % : vends-en ${fmt(round(sell))} et place-les en bourse`,
      tax: { free: false, amount: sell * gainPart * rate },
      level: "warn",
    });
  } else if (share < m.cryptoTarget - band) {
    out.push({ key: "crypto-low", text: `La crypto pèse ${Math.round(share * 100)} %, sous ta cible : tes prochains apports iront d'abord à elle`, level: "info" });
  }

  // Model coins left on platforms, when you have a wallet of your own.
  const k = custody(p, plan);
  if (k.list.some((a) => a.own)) {
    for (const r of cRows) {
      const off = (r.held?.sources ?? [])
        .filter((s) => !ownWallet(plan, s.id, s.name))
        .map((s) => ({ name: s.name, value: num(s.qty) * num(r.held!.price) }))
        .filter((s) => s.value >= 100);
      if (off.length)
        out.push({
          key: "home-" + r.id,
          text: `Envoie tes ${r.name} de ${off.map((s) => s.name).join(" et ")} sur ton portefeuille (${fmt(round(off.reduce((s, x) => s + x.value, 0)))})`,
        });
    }
  }

  // Outside the model, a stock or a coin that weighs too much (an ETF is
  // spread already).
  for (const o of m.out) {
    if (!o.sell && o.cls !== "etf" && o.value / m.invested > plan.cap / 100)
      out.push({ key: "cap-" + o.id, text: `${o.name} pèse ${Math.round((o.value / m.invested) * 100)} %, plus que ta limite de ${plan.cap} %`, level: "warn" });
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

export const crisisLoss = (m: Model, c: (typeof CRISES)[number]) => m.crypto * c.crypto + m.bourse * c.bourse;

/** The lines that weigh, of everything invested: an ETF counts as one line. */
export function concentration(p: Portfolio, invested: number) {
  const lines = p.assets
    .filter(planned)
    .map((h) => ({ id: h.id, name: label(h.asset), value: num(h.value), asset: h.asset, cls: h.class }))
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
  const list = [...by.values()]
    .filter((a) => a.value >= 1)
    .map((a) => ({ ...a, own: ownWallet(plan, a.id, a.name) }))
    .sort((a, b) => b.value - a.value);
  const total = list.reduce((s, a) => s + a.value, 0);
  const mine = list.filter((a) => a.own).reduce((s, a) => s + a.value, 0);
  const btc = list.reduce((s, a) => s + a.btc, 0);
  const btcMine = list.filter((a) => a.own).reduce((s, a) => s + a.btc, 0);
  return { list, total, mine, btc, btcMine };
}

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
