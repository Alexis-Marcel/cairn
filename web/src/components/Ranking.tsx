// Each coin's total gain (held or not), best first: a bar from a middle line,
// right for a gain, left for a loss.

import { Coin } from "./Coin";
import type { CoinStat } from "../api";
import { eur, pct } from "../format";

export function Ranking({ coins, onOpen }: { coins: CoinStat[]; onOpen: (symbol: string) => void }) {
  const list = coins.filter((c) => !c.stable && Math.abs(c.gain) >= 0.5).sort((a, b) => b.gain - a.gain);
  const max = Math.max(1, ...list.map((c) => Math.abs(c.gain)));
  const both = list.some((c) => c.gain < 0) && list.some((c) => c.gain > 0);
  const shown = list.length > 8 ? [...list.slice(0, 4), ...list.slice(-4)] : list;
  return (
    <ol className={both ? "rank both" : "rank"}>
      {shown.map((c) => (
        <li key={c.asset.symbol}>
          <button type="button" onClick={() => onOpen(c.asset.symbol)}>
            <span className="rank-name">
              <Coin symbol={c.asset.symbol} />
              {c.asset.symbol}
            </span>
            <span className="rank-track" aria-hidden>
              <span className={`rank-bar ${c.gain >= 0 ? "up" : "down"}`} style={{ width: `${(Math.abs(c.gain) / max) * (both ? 50 : 100)}%` }} />
            </span>
            <span className={`rank-val num ${c.gain >= 0 ? "up" : "down"}`}>
              {eur(c.gain, { sign: true })}
              {c.pct !== undefined && <small>{pct(c.pct, { sign: true })}</small>}
            </span>
          </button>
        </li>
      ))}
    </ol>
  );
}
