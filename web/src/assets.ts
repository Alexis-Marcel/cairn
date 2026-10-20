// Assets in words: a coin by its name (Bitcoin), a stock or an ETF by the
// name its platform gives it, never by its ISIN, which stays a detail.

import type { Asset, AssetClass } from "./api";
import { COINS } from "./coins";

export const CLASS: Record<AssetClass, string> = {
  crypto: "Crypto",
  stable: "Stablecoins",
  stock: "Actions",
  etf: "ETF",
  cash: "Liquidités",
};

/** One of a class: "Action", "ETF". */
export const CLASS_ONE: Record<AssetClass, string> = {
  crypto: "Crypto",
  stable: "Stablecoin",
  stock: "Action",
  etf: "ETF",
  cash: "Liquidités",
};

/** The ISIN, or symbol, links take: /actifs/IE00B4L5Y983, /actifs/BTC. */
export const assetId = (a: Asset) => (a.kind === "security" && a.isin ? a.isin : a.symbol);

/** What an asset is called: a security's name, a coin's symbol. */
export function label(a: Pick<Asset, "kind" | "symbol" | "name">): string {
  if (a.kind === "security") return a.name || "Titre sans nom";
  return a.symbol;
}

/** A coin's long name (Bitcoin), a security's name. */
export function longName(a: Pick<Asset, "kind" | "symbol" | "name">): string | undefined {
  if (a.kind === "security") return a.name || undefined;
  return COINS.find((c) => c.symbol === a.symbol)?.name;
}

/** Units held: "0,02 BTC", "12,5 parts". */
export const unit = (a: Pick<Asset, "kind" | "symbol">) => (a.kind === "security" ? "parts" : a.symbol);

// Words that say nothing of which fund or company it is.
const STOP = new Set(["the", "core", "msci", "ucits", "etf", "acc", "dist", "s&p", "sa", "se", "ag", "inc", "plc", "nv", "corp", "co", "de", "la", "le", "and", "&"]);

/** Two letters for a security's tile: the first and the last telling words. */
export function monogram(name: string | undefined, fallback: string): string {
  const words = (name ?? "")
    .replace(/\(.*?\)/g, " ")
    .split(/[\s\-–.,/]+/)
    .filter((w) => /[\p{L}\d]/u.test(w));
  const telling = words.filter((w) => !STOP.has(w.toLowerCase()));
  const list = telling.length ? telling : words;
  if (!list.length) return fallback.slice(0, 2).toUpperCase();
  const first = list[0][0];
  if (list.length === 1) return first.toUpperCase();
  return (first + list[list.length - 1][0]).toUpperCase();
}

/** An ISIN's form (its check digit is checked by validISIN). */
export const ISIN = /^[A-Z]{2}[A-Z0-9]{9}\d$/;

/** An ISIN with a right check digit (Luhn over its letters as numbers). */
export function validISIN(s: string): boolean {
  if (!ISIN.test(s)) return false;
  const digits = [...s.slice(0, 11)].flatMap((c) => (c >= "A" ? String(c.charCodeAt(0) - 55).split("") : [c])).map(Number);
  let sum = 0;
  digits.reverse().forEach((d, i) => {
    let x = i % 2 === 0 ? d * 2 : d;
    if (x > 9) x -= 9;
    sum += x;
  });
  return (10 - (sum % 10)) % 10 === Number(s[11]);
}

// Words that name a fund, not a company (as the server's books.Class).
const FUND =
  /\b(etf|etc|etn|ucits|ishares|amundi|lyxor|xtrackers|vanguard|spdr|invesco|wisdomtree|vaneck|ossiam|bnp paribas easy|hsbc msci|ubs etf|franklin ftse|msci|s&p ?500|stoxx|ftse|nasdaq[- ]?100|cac ?40|dax|index|fund|fonds|sicav)\b|\((acc|dist)\)/i;

/** A security's class when the server did not say: by its name, else its ISIN. */
export function securityClass(a: Pick<Asset, "symbol" | "name" | "isin">): "stock" | "etf" {
  if (a.name) return FUND.test(a.name) ? "etf" : "stock";
  return /^(IE|LU)/.test(a.isin ?? a.symbol) ? "etf" : "stock";
}
