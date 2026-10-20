// What a page shows while it loads, or when something did not answer.

import { useEffect, useState } from "react";

export function Failed({ error, retry }: { error: string; retry: () => void }) {
  return (
    <div className="failed" role="alert">
      <p>{error}</p>
      <button type="button" className="btn" onClick={retry}>
        Réessayer
      </button>
    </div>
  );
}

/** A grey shape where something will be. */
export function Skel({ w, h, r, className }: { w?: number | string; h: number | string; r?: number; className?: string }) {
  return <span className={`skel ${className ?? ""}`} style={{ width: w, height: h, borderRadius: r }} aria-hidden />;
}

/** Rows of a list, to come. */
export function SkelRows({ rows = 4 }: { rows?: number }) {
  return (
    <div className="skel-rows" aria-hidden>
      {Array.from({ length: rows }, (_, n) => (
        <div key={n} className="skel-row">
          <Skel w={36} h={36} r={18} />
          <span className="skel-lines">
            <Skel w="40%" h={14} />
            <Skel w="24%" h={12} />
          </span>
          <Skel w={72} h={14} />
        </div>
      ))}
    </div>
  );
}

/** Said only when loading takes a while: the first time, prices are fetched. */
export function Slow({ after = 2500 }: { after?: number }) {
  const [shown, setShown] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => setShown(true), after);
    return () => clearTimeout(t);
  }, [after]);
  return (
    <p className={shown ? "slow on" : "slow"} role="status">
      {shown && (
        <>
          <span className="slow-bar" aria-hidden />
          Récupération des cours, la première fois seulement
        </>
      )}
    </p>
  );
}

export function Loading({ small }: { small?: boolean }) {
  return (
    <div className={small ? "loading small" : "loading"} aria-busy="true" aria-label="Chargement">
      <Skel w="100%" h={small ? 120 : 220} r={16} />
    </div>
  );
}
