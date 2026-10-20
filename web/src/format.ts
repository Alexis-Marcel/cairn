// French formatting: 1 234,56 €, 12 mars 2026. Amounts keep only the
// decimals that say something.

const nf = (o: Intl.NumberFormatOptions) => new Intl.NumberFormat("fr-FR", o);
const MINUS = "−";

// Intl writes a hyphen; a real minus sign lines up with the plus.
const minus = (s: string) => s.replace("-", MINUS);

const euro0 = nf({ style: "currency", currency: "EUR", maximumFractionDigits: 0 });
const euro2 = nf({ style: "currency", currency: "EUR", minimumFractionDigits: 2, maximumFractionDigits: 2 });
const euro0s = nf({ style: "currency", currency: "EUR", maximumFractionDigits: 0, signDisplay: "exceptZero" });
const euro2s = nf({ style: "currency", currency: "EUR", minimumFractionDigits: 2, maximumFractionDigits: 2, signDisplay: "exceptZero" });
const compact = nf({ style: "currency", currency: "EUR", notation: "compact", maximumFractionDigits: 1 });

export const num = (s: string | number | undefined) => (s === undefined ? 0 : typeof s === "number" ? s : Number(s));

/** Euros: cents only under 100 €, unless cents is set. */
export function eur(v: string | number, opts: { cents?: boolean; sign?: boolean } = {}) {
  const n = num(v);
  const cents = opts.cents ?? Math.abs(n) < 100;
  const f = opts.sign ? (cents ? euro2s : euro0s) : cents ? euro2 : euro0;
  return minus(f.format(n));
}

/** Short euros for chart axes: 20 k€. */
export const eurShort = (v: number) => minus(compact.format(v));

/** A quantity of a coin, with the digits that matter for its size. */
export function qty(v: string | number) {
  const n = num(v);
  const a = Math.abs(n);
  if (a === 0) return "0";
  if (a >= 1000) return minus(nf({ maximumFractionDigits: 0 }).format(n));
  if (a >= 1) return minus(nf({ maximumFractionDigits: 3 }).format(n));
  return minus(nf({ maximumSignificantDigits: 4 }).format(n));
}

/** A ratio as a percentage: 58 %, 4,2 %, +32,0 %; a rate keeps its decimal: 31,4 %. */
export function pct(ratio: number, opts: { sign?: boolean; rate?: boolean } = {}) {
  const p = ratio * 100;
  if (opts.rate) return nf({ maximumFractionDigits: 2 }).format(p) + "\u202f%";
  const digits = opts.sign || Math.abs(p) < 10 ? 1 : 0;
  return minus(
    nf({ maximumFractionDigits: digits, minimumFractionDigits: opts.sign ? 1 : 0, signDisplay: opts.sign ? "exceptZero" : "auto" }).format(p),
  ) + " %";
}

const dLong = new Intl.DateTimeFormat("fr-FR", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
const dShort = new Intl.DateTimeFormat("fr-FR", { day: "numeric", month: "short", timeZone: "UTC" });
const dMonth = new Intl.DateTimeFormat("fr-FR", { month: "short", timeZone: "UTC" });
const dMonthYear = new Intl.DateTimeFormat("fr-FR", { month: "short", year: "numeric", timeZone: "UTC" });
const dLocal = new Intl.DateTimeFormat("fr-FR", { day: "numeric", month: "long", year: "numeric" });
const tLocal = new Intl.DateTimeFormat("fr-FR", { hour: "2-digit", minute: "2-digit" });

/** A history day (YYYY-MM-DD, UTC): 12 mars 2026. */
export const day = (t: number) => dLong.format(t);
export const dayShort = (t: number) => dShort.format(t);
export const month = (t: number) => dMonth.format(t);
export const monthYear = (t: number) => dMonthYear.format(t);
/** A moment, in the browser's time zone: 12 mars 2026. */
export const date = (iso: string) => dLocal.format(new Date(iso));
export const time = (iso: string) => tLocal.format(new Date(iso));

/** The CSS class of a signed amount. */
export const tone = (n: number) => (n > 0 ? "up" : n < 0 ? "down" : "");

/** How long ago, briefly: à l'instant, il y a 5 min, il y a 2 h, 12 mars 2026. */
export function ago(iso: string, now = Date.now()) {
  const s = (now - Date.parse(iso)) / 1000;
  if (s < 60) return "à l'instant";
  if (s < 3600) return `il y a ${Math.floor(s / 60)} min`;
  if (s < 86400) return `il y a ${Math.floor(s / 3600)} h`;
  return "le " + date(iso);
}
