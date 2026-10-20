// Actifs: every coin held, then those sold.

import { get, useLoad, type Stats } from "../api";
import { Failed, SkelRows } from "../components/State";
import { CoinList } from "./Home";

export function Assets({ rev, go }: { rev: number; go: (path: string) => void }) {
  const st = useLoad("stats" + rev, (s) => get<Stats>("stats", s));
  const held = st.data?.assets.filter((c) => c.held) ?? [];
  const sold = st.data?.assets.filter((c) => !c.held) ?? [];
  return (
    <div className="assets-page">
      <div className="page-head">
        <h1>Actifs</h1>
      </div>
      {st.error && !st.data ? (
        <Failed error={st.error} retry={st.reload} />
      ) : !st.data ? (
        <SkelRows rows={5} />
      ) : (
        <>
          <CoinList coins={held} go={go} />
          {sold.length > 0 && (
            <section aria-labelledby="h-sold" className="sold">
              <h2 id="h-sold">Plus détenus</h2>
              <CoinList coins={sold} go={go} />
            </section>
          )}
        </>
      )}
    </div>
  );
}
