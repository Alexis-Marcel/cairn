// Cairn's API, under /api. The portfolio, its history and the 2086 have the
// shape of Fundarr's read API (docs/api.md there), computed by Cairn from its
// own activities. Amounts are decimal strings; they are turned into numbers
// only to draw and format them, never summed back into the server's figures.

import { useCallback, useEffect, useState } from "react";

export interface Asset {
  kind: "crypto" | "fiat" | "security";
  symbol: string;
  isin?: string;
  name?: string;
}

/** crypto, stablecoin, stock, ETF (and fund), cash. */
export type AssetClass = "crypto" | "stable" | "stock" | "etf" | "cash";

/** An asset the operations use (GET /api/assets), most used first. id: ISIN or symbol. */
export interface AssetRef {
  id: string;
  symbol: string;
  kind: Asset["kind"];
  name?: string;
  class: AssetClass;
}

export interface Amount {
  asset: Asset;
  qty: string;
}

export interface SourceRef {
  id: string;
  name: string;
  kind?: string;
}

export interface Activity {
  id: string;
  source: SourceRef;
  account: string;
  time: string;
  type: "trade" | "deposit" | "withdrawal" | "reward" | "fee" | "dividend" | "interest" | "tax" | "adjustment";
  side?: "buy" | "sell";
  base?: Amount;
  quote?: Amount;
  fee?: Amount;
  cost?: string;
  currency?: string;
  transferWith?: string;
  correction?: boolean;
}

export interface Activities {
  activities: Activity[];
  next: string;
}

export interface Portfolio {
  currency: string;
  at: string;
  total: {
    value: string;
    invested: string;
    cost: string;
    unrealized: string;
    realized: string;
    rewards: string;
    /** Cash on the accounts, part of the value. */
    cash: string;
  };
  /** Each account's cash, by currency. */
  cash: (SourceRef & { asset: Asset; qty: string; value: string })[];
  assets: {
    asset: Asset;
    qty: string;
    price: string;
    value: string;
    cost: string;
    unrealized: string;
    share: string;
    sources: { id: string; name: string; qty: string }[];
    id: string;
    class: AssetClass;
  }[];
  sources: (SourceRef & { value: string })[];
  unpriced: Asset[];
}

export interface History {
  currency: string;
  points: { date: string; value: string; invested: string }[];
  unpriced?: Asset[];
}

export interface Disposal {
  time: string;
  source: SourceRef;
  asset: Asset;
  qty: string;
  price: string;
  fees: string;
  portfolio: string;
  acquisition: string;
  capitalShare: string;
  gain: string;
}

export interface TaxFR {
  year: number;
  currency: string;
  disposals: Disposal[];
  total: { disposals: string; gain: string; exempt: boolean; tax: string; rate: string };
  notes: string[];
}

/** An answer of Cairn's server that went wrong, with its message. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status = 0,
  ) {
    super(message);
  }
}

export async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  let resp: Response;
  try {
    resp = await fetch("/api/" + path, { signal, headers: { Accept: "application/json" } });
  } catch (e) {
    if (signal?.aborted) throw e;
    throw new ApiError("Cairn est injoignable");
  }
  if (!resp.ok) {
    const body = await resp.json().catch(() => null);
    throw new ApiError(body?.error ?? `Erreur ${resp.status}`, resp.status);
  }
  return resp.json();
}

export interface Loaded<T> {
  data?: T;
  error?: string;
  loading: boolean;
  reload: () => void;
}

/**
 * Loads with load, again whenever key changes (null: not yet). The last data
 * stays shown while the next loads, so switching a year does not blank the page.
 */
export function useLoad<T>(key: string | null, load: (signal: AbortSignal) => Promise<T>): Loaded<T> {
  const [state, setState] = useState<{ data?: T; error?: string; loading: boolean }>({ loading: true });
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (key === null) return;
    const ctl = new AbortController();
    setState((s) => ({ data: s.data, loading: true }));
    load(ctl.signal).then(
      (data) => setState({ data, loading: false }),
      (e) => {
        if (!ctl.signal.aborted) setState((s) => ({ data: s.data, error: e instanceof Error ? e.message : String(e), loading: false }));
      },
    );
    return () => ctl.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, tick]);
  const reload = useCallback(() => setTick((t) => t + 1), []);
  return { ...state, reload };
}

/** The link to Fundarr (the token never comes back). */
export interface SyncState {
  linked: boolean;
  fromEnv: boolean;
  url?: string;
  lastSync?: string;
  lastError?: string;
  notify: boolean;
  running: boolean;
}

export interface Status {
  activities: number;
  sync: SyncState;
  /** The history holds crypto and securities: the scopes say something. */
  scopes: boolean;
}

/** What the figures cover: everything, crypto alone, stocks and ETFs alone. */
export type Scope = "all" | "crypto" | "securities";
export const SCOPES = ["all", "crypto", "securities"] as const;

export interface Account {
  id: string;
  name: string;
  origin: "manual" | "fundarr";
  count: number;
}

export type Kind = "buy" | "sell" | "swap" | "deposit" | "withdrawal" | "reward" | "dividend" | "interest" | "transfer" | "fee" | "correction";
export type Origin = "manual" | "import" | "fundarr";

/** An operation, as the Opérations page lists it. */
export interface Op {
  id: string;
  origin: Origin;
  kind: Kind;
  type: Activity["type"];
  time: string;
  account: SourceRef;
  side?: "buy" | "sell" | "in" | "out";
  base: Amount;
  quote?: Amount;
  fee?: Amount;
  value?: string;
  note?: string;
  peer?: { id: string; account: SourceRef; base: Amount; fee?: Amount };
  linked?: boolean;
  editable: boolean;
}

export interface Ops {
  items: Op[];
  total: number;
  offset: number;
  limit: number;
}

/** An operation as the form sends it. */
export interface Entry {
  kind: Exclude<Kind, "correction">;
  date: string;
  account?: string;
  newAccount?: string;
  to?: string;
  newTo?: string;
  asset: string;
  name?: string;
  qty: string;
  total?: string;
  fee?: string;
  received?: string;
  receivedQty?: string;
  note?: string;
}

export interface Preview {
  format: "fundarr" | "cairn";
  rows: {
    line: number;
    kind?: Kind;
    time?: string;
    account?: string;
    base?: Amount;
    quote?: Amount;
    error?: string;
    duplicate?: boolean;
  }[];
  total: number;
  valid: number;
  errors: number;
  duplicates: number;
  fileAccounts: boolean;
  accounts: string[];
}

/** A write: JSON both ways, the server's message on failure. */
export async function send<T>(method: "POST" | "PUT" | "PATCH" | "DELETE", path: string, body: unknown = {}): Promise<T> {
  let resp: Response;
  try {
    resp = await fetch("/api/" + path, {
      method,
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(body),
    });
  } catch {
    throw new ApiError("Cairn est injoignable");
  }
  const data = await resp.json().catch(() => null);
  if (!resp.ok) throw new ApiError(data?.error ?? `Erreur ${resp.status}`, resp.status);
  return data;
}

export const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

export type PeriodKey = "7J" | "1M" | "YTD" | "1A" | "Tout";

/** GET /api/overview: the value day by day, each period's change, the totals. */
export interface Overview {
  currency: string;
  at: string;
  step: "day" | "week";
  /** d: day, v: value, i: invested, x: time-weighted index (1 at the start). */
  points: { d: string; v: number; i: number; x: number }[];
  /** gain: beyond money put in or taken out; pct: gain over what was at stake (null when nothing was); twr: time-weighted. */
  periods: { key: PeriodKey; from: number; start: number; end: number; gain: number; pct: number | null; twr: number }[];
  total: {
    value: number;
    /** Every external flow, the adjustments among them: what the gain is measured against. */
    invested: number;
    gain: number;
    realized: number;
    unrealized: number;
    rewards: number;
    /** Paid by securities, net of tax; part of the gain. */
    dividends: number;
    /** On cash; part of the gain. */
    interest: number;
    fees: number;
    stable: number;
    xirr: number | null;
    /** Money put in and taken out from outside, and the net. */
    paidIn: number;
    paidOut: number;
    contributed: number;
    /** Fundarr's balance corrections: neither money put in nor gains. */
    adjusted: number;
    /** Cash on the accounts (Liquidités). */
    cash: number;
  };
  /** The value by class of asset. */
  classes: Partial<Record<AssetClass, number>>;
  unpriced: Asset[];
}

/** One coin's results. */
export interface CoinStat {
  asset: Asset;
  qty: string;
  price: number;
  value: number;
  cost: number;
  avgCost: number;
  share: number;
  unrealized: number;
  realized: number;
  rewards: number;
  dividends: number;
  fees: number;
  gain: number;
  pct?: number;
  held: boolean;
  stable?: boolean;
  first: string;
  spark?: number[];
  /** ISIN or symbol: what links take. */
  id: string;
  class: AssetClass;
}

/** GET /api/stats: month by month, and each coin. */
export interface Stats {
  at: string;
  months: {
    m: string;
    ret: number;
    /** Money put in and taken out; adj: Fundarr's corrections (signed). */
    in: number;
    out: number;
    adj: number;
    rewards: number;
    dividends: number;
    interest: number;
    value: number;
    /** Every external flow so far; adjusted: the corrections among it. */
    invested: number;
    adjusted: number;
    earned: number;
  }[];
  assets: CoinStat[];
  /** Cash by account, largest first. */
  cash: CashStat[];
}

/** An account's cash. share: of the value. */
export interface CashStat {
  account: SourceRef;
  value: number;
  share: number;
  held: { symbol: string; qty: string }[];
}

/** GET /api/assets/{symbol}: one coin day by day. */
export interface AssetDetail {
  at: string;
  step: "day" | "week";
  coin: CoinStat;
  points: { d: string; p: number; q: number; avg?: number }[];
  markers: { d: string; buy: boolean; qty: string; value: number; price: number }[];
}
