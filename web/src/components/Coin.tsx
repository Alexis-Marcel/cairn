// An asset's icon: a coin's (cryptocurrency-icons, CC0) or its initial on a
// pebble; a stock or an ETF as a tile with its monogram, tinted by class.

import type { Asset, AssetClass } from "../api";
import { monogram, securityClass } from "../assets";

const icons = import.meta.glob<string>("../icons/*.svg", { eager: true, query: "?url", import: "default" });

const bySymbol: Record<string, string> = {};
for (const [path, url] of Object.entries(icons)) bySymbol[path.slice(9, -4).toUpperCase()] = url;

export function Coin({
  symbol,
  asset,
  cls,
  size = 24,
}: {
  symbol?: string;
  asset?: Pick<Asset, "kind" | "symbol" | "name" | "isin">;
  cls?: AssetClass;
  size?: number;
}) {
  const sym = asset?.symbol ?? symbol ?? "";
  const style = { width: size, height: size };
  if (asset?.kind === "security") {
    return (
      <span className={`coin tile ${cls ?? securityClass(asset)}`} style={{ ...style, fontSize: size * 0.36 }} aria-hidden>
        {monogram(asset.name, sym)}
      </span>
    );
  }
  const url = bySymbol[sym.toUpperCase()];
  if (url) return <img className="coin" src={url} alt="" width={size} height={size} style={style} />;
  return (
    <span className="coin coin-none" style={{ ...style, fontSize: size * 0.42 }} aria-hidden>
      {sym.toUpperCase() === "EUR" ? "€" : sym.slice(0, 1)}
    </span>
  );
}
