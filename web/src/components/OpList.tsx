// Operations as a list, grouped by month, newest first: what happened, on
// which account and day; what it moved and was worth.

import type { Op } from "../api";
import { dayOf, eur, monthLong, num } from "../format";
import { label } from "../assets";
import { KIND, detail, moves } from "../ops";
import { Coin } from "./Coin";

/** sameAsset: every operation is about one asset (its page): its name is not repeated. */
export function OpList({ items, onOpen, sameAsset }: { items: Op[]; onOpen: (op: Op) => void; sameAsset?: boolean }) {
  const groups: { key: string; label: string; ops: Op[] }[] = [];
  for (const op of items) {
    const d = new Date(op.time);
    const key = `${d.getFullYear()}-${d.getMonth()}`;
    if (groups[groups.length - 1]?.key !== key) groups.push({ key, label: monthLong(Date.UTC(d.getFullYear(), d.getMonth(), 1)), ops: [] });
    groups[groups.length - 1].ops.push(op);
  }
  return (
    <div className="op-list">
      {groups.map((g) => (
        <section key={g.key} aria-label={g.label}>
          <h3 className="op-month">{g.label}</h3>
          <ul>
            {g.ops.map((op) => {
              const m = moves(op);
              return (
                <li key={op.id}>
                  <button type="button" className="op-row" onClick={() => onOpen(op)}>
                    <Coin asset={m.asset} size={40} />
                    <span className="op-main">
                      <span className="op-title-line">
                        {op.type === "tax" ? "Impôt" : KIND[op.kind]}
                        {m.asset.kind !== "fiat" && !sameAsset && <span className="op-sym">{label(m.asset)}</span>}
                      </span>
                      <span className="op-sub">
                        {dayOf(op.time)} · {detail(op)}
                      </span>
                    </span>
                    <span className="op-amounts num">
                      <span className="op-qty">{m.text}</span>
                      <span className="op-val">{m.other ?? (op.value && op.base.asset.kind !== "fiat" ? eur(num(op.value)) : "")}</span>
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </section>
      ))}
    </div>
  );
}
