// Where the value sits: one bar of stones, widths by share, and its key.

import { eur, pct } from "../format";

export interface Part {
  key: string;
  label: string;
  value: number;
}

/** Up to five parts; the rest is grouped. */
function group(parts: Part[], total: number): Part[] {
  const sorted = parts.filter((p) => p.value > 0).sort((a, b) => b.value - a.value);
  if (sorted.length <= 6) return sorted;
  const head = sorted.slice(0, 5);
  const rest = sorted.slice(5).reduce((s, p) => s + p.value, 0);
  return rest / total > 0 ? [...head, { key: "_rest", label: "Autres", value: rest }] : head;
}

export function Allocation({ parts }: { parts: Part[] }) {
  const total = parts.reduce((s, p) => s + p.value, 0);
  const shown = group(parts, total);
  if (!total) return null;
  return (
    <div className="alloc">
      <div className="alloc-bar" aria-hidden>
        {shown.map((p, n) => (
          <span key={p.key} className={`c${n}`} style={{ flexGrow: p.value / total }} />
        ))}
      </div>
      <ul className="alloc-key">
        {shown.map((p, n) => (
          <li key={p.key}>
            <span className={`sw c${n}`} aria-hidden />
            <span className="name">{p.label}</span>
            <span className="num share">{pct(p.value / total)}</span>
            <span className="num muted">{eur(p.value)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
