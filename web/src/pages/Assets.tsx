// Actifs: every coin and security held, then those sold. A long list can be
// searched and sorted.

import { useMemo, useState } from "react";
import { get, useLoad, type CoinStat, type Stats } from "../api";
import { label, longName } from "../assets";
import { ScopeSwitch } from "../components/ScopeSwitch";
import { Segmented } from "../components/Segmented";
import { Failed, SkelRows } from "../components/State";
import { remember, remembered } from "../lib/remember";
import { CoinList, type Scoped } from "./Home";

type Sort = "value" | "gain" | "name";
const SORTS: { value: Sort; label: string }[] = [
  { value: "value", label: "Valeur" },
  { value: "gain", label: "Gain" },
  { value: "name", label: "Nom" },
];

// Search and sorting help from this many assets on.
const LONG = 9;

const fold = (s: string) =>
  s
    .normalize("NFD")
    .replace(/\p{M}/gu, "")
    .toLowerCase();

function sorted(list: CoinStat[], by: Sort): CoinStat[] {
  if (by === "value") return list;
  const out = [...list];
  if (by === "gain") out.sort((a, b) => (b.held ? b.unrealized : b.gain) - (a.held ? a.unrealized : a.gain));
  else out.sort((a, b) => label(a.asset).localeCompare(label(b.asset), "fr", { sensitivity: "base" }));
  return out;
}

export function Assets({ rev, go, scope, onScope }: { rev: number; go: (path: string) => void } & Scoped) {
  const st = useLoad("stats?scope=" + scope + rev, (s) => get<Stats>("stats?scope=" + scope, s));
  const [q, setQ] = useState("");
  const [by, setBy] = useState<Sort>(() => remembered("assetSort", "value", ["value", "gain", "name"] as const));
  const all = useMemo(() => st.data?.assets ?? [], [st.data]);
  const long = all.length >= LONG;

  const { held, sold } = useMemo(() => {
    const words = fold(q).split(/\s+/).filter(Boolean);
    const hit = (c: CoinStat) => {
      if (!words.length) return true;
      const text = fold([label(c.asset), longName(c.asset) ?? "", c.asset.symbol, c.asset.isin ?? ""].join(" "));
      return words.every((w) => text.includes(w));
    };
    const list = all.filter(hit);
    return { held: sorted(list.filter((c) => c.held), by), sold: sorted(list.filter((c) => !c.held), by) };
  }, [all, q, by]);

  return (
    <div className="assets-page">
      <div className="page-head">
        <h1>Actifs</h1>
        {onScope && <ScopeSwitch scope={scope} onChange={onScope} />}
      </div>
      {long && (
        <div className="filters">
          <label className="search">
            <svg viewBox="0 0 16 16" aria-hidden>
              <circle cx="7" cy="7" r="4.5" />
              <path d="M10.5 10.5 14 14" />
            </svg>
            <span className="sr">Rechercher un actif</span>
            <input type="search" placeholder="Rechercher" value={q} onChange={(e) => setQ(e.target.value)} />
          </label>
          <Segmented
            label="Trier par"
            className="toggle small"
            options={SORTS}
            value={by}
            onChange={(v) => {
              setBy(v);
              remember("assetSort", v);
            }}
          />
        </div>
      )}
      {st.error && !st.data ? (
        <Failed error={st.error} retry={st.reload} />
      ) : !st.data ? (
        <SkelRows rows={5} />
      ) : held.length + sold.length === 0 ? (
        <p className="empty">{q ? "Aucun actif ne correspond." : "Aucun actif."}</p>
      ) : (
        <div className={st.loading ? "stale" : ""}>
          {held.length > 0 && <CoinList coins={held} go={go} />}
          {sold.length > 0 && (
            <section aria-labelledby="h-sold" className="sold">
              <h2 id="h-sold">Plus détenus</h2>
              <CoinList coins={sold} go={go} />
            </section>
          )}
        </div>
      )}
    </div>
  );
}
