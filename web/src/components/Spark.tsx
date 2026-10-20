// A month of a coin's price, as a stroke: up or down from its first day.

import { smooth } from "../lib/chart";

export function Spark({ values, width = 72, height = 28 }: { values: number[]; width?: number; height?: number }) {
  if (values.length < 2) return <span className="spark" style={{ width, height }} aria-hidden />;
  const lo = Math.min(...values);
  const hi = Math.max(...values);
  const xs = values.map((_, i) => (i / (values.length - 1)) * width);
  const ys = values.map((v) => 3 + (height - 6) * (1 - (v - lo) / (hi - lo || 1)));
  const up = values[values.length - 1] >= values[0];
  return (
    <svg className={`spark ${up ? "up" : "down"}`} width={width} height={height} viewBox={`0 0 ${width} ${height}`} aria-hidden>
      <path d={smooth(xs, ys)} />
    </svg>
  );
}
