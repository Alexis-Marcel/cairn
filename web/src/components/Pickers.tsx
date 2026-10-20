// The two pickers of the form: an account (or a new one, typed in place),
// and a coin (searched by symbol or name, with its icon).

import { useId, useMemo, useRef, useState } from "react";
import type { Account } from "../api";
import { COINS, EUR, searchCoins, type CoinInfo } from "../coins";
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

export function AssetPicker({
  label,
  value,
  onChange,
  used = [],
  cash,
  autoFocus,
}: {
  label: string;
  value: string;
  onChange: (symbol: string) => void;
  used?: string[];
  cash?: boolean;
  autoFocus?: boolean;
}) {
  const id = useId();
  const [text, setText] = useState(value);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const input = useRef<HTMLInputElement>(null);

  const all = useMemo(() => {
    const known = new Set(COINS.map((c) => c.symbol));
    const extra: CoinInfo[] = used.filter((s) => !known.has(s) && s !== "EUR").map((s) => ({ symbol: s, name: "" }));
    // What is already held comes first.
    const mine = COINS.filter((c) => used.includes(c.symbol));
    const others = COINS.filter((c) => !used.includes(c.symbol));
    return [...(cash ? [EUR] : []), ...mine, ...extra, ...others];
  }, [used, cash]);

  const typed = text.trim().toUpperCase();
  const matches = useMemo(() => {
    const m = searchCoins(text, all).slice(0, 8);
    if (typed && /^[A-Z0-9][A-Z0-9.]{0,19}$/.test(typed) && !all.some((c) => c.symbol === typed)) m.push({ symbol: typed, name: "" });
    return m;
  }, [text, all, typed]);

  const pick = (c: CoinInfo) => {
    setText(c.symbol);
    onChange(c.symbol);
    setOpen(false);
  };
  const shown = open && matches.length > 0;

  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      <div className="combo">
        {value && value === typed && (
          <span className="combo-coin" aria-hidden>
            <Coin symbol={value} />
          </span>
        )}
        <input
          ref={input}
          id={id}
          type="text"
          role="combobox"
          aria-expanded={shown}
          aria-controls={id + "-list"}
          aria-autocomplete="list"
          aria-activedescendant={shown ? `${id}-o${active}` : undefined}
          className={value && value === typed ? "has-coin" : ""}
          placeholder="BTC, Ethereum…"
          autoComplete="off"
          autoCapitalize="characters"
          spellCheck={false}
          autoFocus={autoFocus}
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            setOpen(true);
            setActive(0);
            const t = e.target.value.trim().toUpperCase();
            onChange(all.some((c) => c.symbol === t) ? t : "");
          }}
          onFocus={() => setOpen(true)}
          onBlur={() => {
            setOpen(false);
            // A symbol typed out counts, known or not.
            if (typed && /^[A-Z0-9][A-Z0-9.]{0,19}$/.test(typed)) {
              setText(typed);
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
            } else if (e.key === "Enter" && shown) {
              e.preventDefault();
              pick(matches[active]);
            } else if (e.key === "Escape" && shown) {
              e.preventDefault();
              e.stopPropagation();
              setOpen(false);
            }
          }}
        />
        {shown && (
          <ul className="combo-list" id={id + "-list"} role="listbox" aria-label={label}>
            {matches.map((c, n) => (
              <li
                key={c.symbol}
                id={`${id}-o${n}`}
                role="option"
                aria-selected={n === active}
                className={n === active ? "on" : ""}
                onMouseDown={(e) => {
                  e.preventDefault();
                  pick(c);
                }}
                onMouseEnter={() => setActive(n)}
              >
                <Coin symbol={c.symbol} />
                <span className="sym">{c.symbol}</span>
                <span className="muted">{c.name}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
