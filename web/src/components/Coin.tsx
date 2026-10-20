// A coin's icon (cryptocurrency-icons, CC0), or its initial on a pebble.

const icons = import.meta.glob<string>("../icons/*.svg", { eager: true, query: "?url", import: "default" });

const bySymbol: Record<string, string> = {};
for (const [path, url] of Object.entries(icons)) bySymbol[path.slice(9, -4).toUpperCase()] = url;

export function Coin({ symbol }: { symbol: string }) {
  const url = bySymbol[symbol.toUpperCase()];
  if (url) return <img className="coin" src={url} alt="" width={22} height={22} />;
  return (
    <span className="coin coin-none" aria-hidden>
      {symbol.toUpperCase() === "EUR" ? "€" : symbol.slice(0, 1)}
    </span>
  );
}
