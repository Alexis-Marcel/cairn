// A figure that rolls to its new value instead of jumping, unless asked to
// follow at once (scrubbing) or motion is reduced.

import { useEffect, useRef, useState } from "react";
import { ease, reducedMotion } from "../lib/chart";

export function useTween(value: number, instant = false, ms = 560): number {
  const [shown, setShown] = useState(value);
  const cur = useRef(value);
  useEffect(() => {
    if (instant || reducedMotion() || !Number.isFinite(cur.current)) {
      cur.current = value;
      setShown(value);
      return;
    }
    const from = cur.current;
    if (from === value) return;
    let raf = 0;
    const start = performance.now();
    const step = (now: number) => {
      const t = Math.min(1, (now - start) / ms);
      cur.current = from + (value - from) * ease(t);
      setShown(cur.current);
      if (t < 1) raf = requestAnimationFrame(step);
    };
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [value, instant, ms]);
  return shown;
}

export function Num({ value, format, instant, className }: { value: number; format: (v: number) => string; instant?: boolean; className?: string }) {
  const v = useTween(value, instant);
  return <span className={className}>{format(v)}</span>;
}
