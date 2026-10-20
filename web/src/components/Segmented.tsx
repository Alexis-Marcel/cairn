// A few mutually exclusive choices, as one row of buttons; the arrow keys
// move between them.

import { useRef } from "react";

export function Segmented<T extends string | number>({
  label,
  options,
  value,
  onChange,
  className,
}: {
  label: string;
  options: { value: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const key = (e: React.KeyboardEvent) => {
    const i = options.findIndex((o) => o.value === value);
    const to = { ArrowLeft: i - 1, ArrowRight: i + 1, ArrowUp: i - 1, ArrowDown: i + 1 }[e.key];
    if (to === undefined) return;
    e.preventDefault();
    const n = (to + options.length) % options.length;
    onChange(options[n].value);
    (ref.current?.children[n] as HTMLElement | undefined)?.focus();
  };
  return (
    <div className={`seg ${className ?? ""}`} role="radiogroup" aria-label={label} ref={ref} onKeyDown={key}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={o.value === value}
          tabIndex={o.value === value ? 0 : -1}
          className={o.value === value ? "on" : ""}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
