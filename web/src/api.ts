// Cairn's API, under /api. The portfolio, its history and the 2086 have the
// shape of Fundarr's read API (docs/api.md there), computed by Cairn from its
// own activities. Amounts are decimal strings; they are turned into numbers
// only to draw and format them, never summed back into the server's figures.

import { useCallback, useEffect, useState } from "react";

export interface Asset {
  kind: "crypto" | "fiat" | "security";
  symbol: string;
  name?: string;
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
  };
  assets: {
    asset: Asset;
    qty: string;
    price: string;
    value: string;
    cost: string;
    unrealized: string;
    share: string;
    sources: { id: string; name: string; qty: string }[];
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

/**
 * The history, one point a day; a week when that is too many for Fundarr
 * (it answers 422 past 3700 points, ten years of days).
 */
export async function history(signal?: AbortSignal): Promise<History> {
  try {
    return await get<History>("portfolio/history?step=day", signal);
  } catch (e) {
    if (e instanceof ApiError && e.status === 422) return get<History>("portfolio/history?step=week", signal);
    throw e;
  }
}

/** Every activity since a time, following the pages. */
export async function activitiesSince(since: Date, signal?: AbortSignal): Promise<Activity[]> {
  const out: Activity[] = [];
  let after = "";
  for (let page = 0; page < 20; page++) {
    const q = new URLSearchParams({ since: since.toISOString(), limit: "5000" });
    if (after) q.set("after", after);
    const r = await get<Activities>("activities?" + q, signal);
    out.push(...r.activities);
    if (!r.next || r.activities.length === 0) break;
    after = r.next;
  }
  return out;
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
}

export interface Account {
  id: string;
  name: string;
  origin: "manual" | "fundarr";
  count: number;
}

export type Kind = "buy" | "sell" | "swap" | "deposit" | "withdrawal" | "reward" | "transfer" | "fee" | "correction";
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
