// A coin's icon (cryptocurrency-icons, CC0), or its initial on a pebble.

const icons = import.meta.glob<string>("../icons/*.svg", { eager: true, query: "?url", import: "default" });

const bySymbol: Record<string, string> = {};
for (const [path, url] of Object.entries(icons)) bySymbol[path.slice(9, -4).toUpperCase()] = url;

export function Coin({ symbol, size = 24 }: { symbol: string; size?: number }) {
  const url = bySymbol[symbol.toUpperCase()];
  const style = { width: size, height: size };
  if (url) return <img className="coin" src={url} alt="" width={size} height={size} style={style} />;
  return (
    <span className="coin coin-none" style={{ ...style, fontSize: size * 0.42 }} aria-hidden>
      {symbol.toUpperCase() === "EUR" ? "€" : symbol.slice(0, 1)}
    </span>
  );
}
