// Fundarr's balance corrections, said quietly: their amount, and why on
// demand. They are neither money put in nor a gain.

import { eur } from "../format";
import { ADJUST_WHY } from "../ops";

export function Adjusted({ amount, className = "" }: { amount: number; className?: string }) {
  return (
    <details className={`adjusted ${className}`}>
      <summary title={ADJUST_WHY}>
        Ajustements Fundarr <span className="num nowrap">{eur(amount, { sign: true })}</span>
        <svg viewBox="0 0 16 16" aria-hidden className="info">
          <circle cx="8" cy="8" r="6.25" />
          <path d="M8 7.2v4M8 4.8v.01" />
        </svg>
        <span className="sr">, pourquoi</span>
      </summary>
      <p>{ADJUST_WHY}</p>
    </details>
  );
}
