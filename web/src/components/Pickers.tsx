// The two pickers of the form: an account (or a new one, typed in place),
// and an asset (a coin by symbol or name, a stock or ETF by name or ISIN,
// with its icon).

import { useId, useMemo, useRef, useState } from "react";
import type { Account, AssetRef } from "../api";
import { ISIN } from "../assets";
import { COINS, EUR } from "../coins";
import { Coin } from "./Coin";

/** An account chosen: an existing one (id) or a new one (name). */
export interface AccountChoice {
  id?: string;
  name?: string;
}

const NEW = "__new";

export function AccountPicker({
  label,
  accounts,
  value,
  onChange,
  exclude,
}: {
  label: string;
  accounts: Account[];
  value: AccountChoice;
  onChange: (v: AccountChoice) => void;
  exclude?: string;
}) {
  const id = useId();
  const list = accounts.filter((a) => a.id !== exclude);
  const typing = value.name !== undefined || list.length === 0;
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {typing ? (
        <div className="with-action">
          <input
            id={id}
            type="text"
            placeholder="Binance, Ledger…"
            autoComplete="off"
            maxLength={60}
            value={value.name ?? ""}
            autoFocus={list.length > 0}
            onChange={(e) => onChange({ name: e.target.value })}
          />
          {list.length > 0 && (
            <button type="button" className="btn quiet small" onClick={() => onChange({ id: list[0].id })}>
              Choisir
            </button>
          )}
        </div>
      ) : (
        <select
          id={id}
          value={value.id ?? ""}
          onChange={(e) => onChange(e.target.value === NEW ? { name: "" } : { id: e.target.value })}
        >
          {!value.id && <option value="">Choisir…</option>}
          {list.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
          <option value={NEW}>Nouveau compte…</option>
        </select>
      )}
    </div>
  );
}

/** An asset to pick: a coin, cash, or a stock or ETF by its ISIN. */
interface Option {
  id: string; // symbol, or ISIN
  kind: AssetRef["kind"];
  name: string;
  cls?: AssetRef["class"];
}

const SYMBOL = /^[A-Z0-9][A-Z0-9.]{0,19}$/;

/** What the input shows for an option: a coin's symbol, a security's name. */
const shown = (o: Option) => (o.kind === "security" && o.name ? o.name : o.id);

/**
 * A coin, cash or a security, searched by symbol, name or ISIN. Securities
 * offered are those already used; any other is typed by its ISIN (checked).
 * value is the symbol or ISIN.
 */
export function AssetPicker({
  label,
  value,
  onChange,
  used = [],
  cash,
  coins = true,
  securities = true,
  newName,
  autoFocus,
}: {
  label: string;
  value: string;
  onChange: (id: string) => void;
  used?: AssetRef[];
  cash?: boolean;
  coins?: boolean;
  securities?: boolean;
  /** The name typed for a security Cairn does not know yet, for its icon. */
  newName?: string;
  autoFocus?: boolean;
}) {
  const id = useId();
  const all = useMemo(() => {
    const mine: Option[] = used
      .filter((a) => (a.kind === "security" ? securities : coins && a.kind === "crypto"))
      .map((a) => ({ id: a.id, kind: a.kind, name: a.name || COINS.find((c) => c.symbol === a.symbol)?.name || "", cls: a.class }));
    const known = new Set(mine.map((o) => o.id));
    const others: Option[] = coins ? COINS.filter((c) => !known.has(c.symbol)).map((c) => ({ id: c.symbol, kind: "crypto", name: c.name })) : [];
    return [...(cash ? [{ id: EUR.symbol, kind: "fiat" as const, name: EUR.name }] : []), ...mine, ...others];
  }, [used, cash, coins, securities]);
  const byId = (v: string) => all.find((o) => o.id === v);
  const [text, setText] = useState(() => {
    const o = byId(value);
    return o ? shown(o) : value;
  });
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const input = useRef<HTMLInputElement>(null);

  const typed = text.trim().toUpperCase();
  const matches = useMemo(() => {
    const m = search(text, all).slice(0, 8);
    const isin = ISIN.test(typed);
    if (typed && !all.some((o) => o.id === typed)) {
      if (isin && securities) m.push({ id: typed, kind: "security", name: "" });
      else if (!isin && coins && SYMBOL.test(typed)) m.push({ id: typed, kind: "crypto", name: "" });
    }
    return m;
  }, [text, all, typed, coins, securities]);

  const pick = (o: Option) => {
    setText(shown(o));
    onChange(o.id);
    setOpen(false);
  };
  const current = byId(value) ?? (value ? { id: value, kind: ISIN.test(value) ? ("security" as const) : ("crypto" as const), name: "" } : null);
  const showsIcon = !!current && shown(current).toUpperCase() === typed;
  const listed = open && matches.length > 0;

  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      <div className="combo">
        {showsIcon && (
          <span className="combo-coin" aria-hidden>
            <Coin asset={{ kind: current.kind, symbol: current.id, name: current.name || newName }} cls={current.cls} />
          </span>
        )}
        <input
          ref={input}
          id={id}
          type="text"
          role="combobox"
          aria-expanded={listed}
          aria-controls={id + "-list"}
          aria-autocomplete="list"
          aria-activedescendant={listed ? `${id}-o${active}` : undefined}
          className={showsIcon ? "has-coin" : ""}
          placeholder={!securities ? "BTC, Ethereum…" : coins ? "BTC, nom ou ISIN…" : "Nom ou ISIN"}
          autoComplete="off"
          spellCheck={false}
          autoFocus={autoFocus}
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            setOpen(true);
            setActive(0);
            const t = e.target.value.trim();
            const o = all.find((x) => x.id === t.toUpperCase() || (x.kind === "security" && x.name && x.name === t));
            onChange(o ? o.id : "");
          }}
          onFocus={() => setOpen(true)}
          onBlur={() => {
            setOpen(false);
            if (current && shown(current) === text.trim()) return;
            // A symbol or an ISIN typed out counts, known or not.
            if (typed && ((ISIN.test(typed) && securities) || (!ISIN.test(typed) && coins && SYMBOL.test(typed)))) {
              const o = byId(typed);
              setText(o ? shown(o) : typed);
              onChange(typed);
            }
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setOpen(true);
              setActive((a) => Math.min(a + 1, matches.length - 1));
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setActive((a) => Math.max(a - 1, 0));
            } else if (e.key === "Enter" && listed) {
              e.preventDefault();
              pick(matches[active]);
            } else if (e.key === "Escape" && listed) {
              e.preventDefault();
              e.stopPropagation();
              setOpen(false);
            }
          }}
        />
        {listed && (
          <ul className="combo-list" id={id + "-list"} role="listbox" aria-label={label}>
            {matches.map((o, n) => (
              <li
                key={o.id}
                id={`${id}-o${n}`}
                role="option"
                aria-selected={n === active}
                className={n === active ? "on" : ""}
                onMouseDown={(e) => {
                  e.preventDefault();
                  pick(o);
                }}
                onMouseEnter={() => setActive(n)}
              >
                <Coin asset={{ kind: o.kind, symbol: o.id, name: o.name }} cls={o.cls} />
                {o.kind === "security" ? (
                  <>
                    <span className="sym name">{o.name || "Nouveau titre"}</span>
                    <span className="muted">{o.id}</span>
                  </>
                ) : (
                  <>
                    <span className="sym">{o.id}</span>
                    <span className="muted">{o.name}</span>
                  </>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

const fold = (s: string) =>
  s
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase();

/** Options matching what is typed: symbol or ISIN first, then name. */
function search(q: string, list: Option[]): Option[] {
  const t = fold(q.trim());
  if (!t) return list;
  const starts = list.filter((o) => fold(o.id).startsWith(t) || (o.kind === "security" && fold(o.name).startsWith(t)));
  const rest = list.filter((o) => !starts.includes(o) && (fold(o.name).includes(t) || fold(o.id).includes(t)));
  return [...starts, ...rest];
}
