// Activité: every operation, newest first, whatever its origin. Filter by
// account or coin, search, open one to change it.

import { useEffect, useMemo, useState } from "react";
import { get, useLoad, type Account, type Op, type Ops } from "../api";
import { OpList } from "../components/OpList";
import { Failed, SkelRows } from "../components/State";

const PAGE = 50;

export function Activity({ rev, onOpen, onImport }: { rev: number; onOpen: (op: Op) => void; onImport: () => void }) {
  const [account, setAccount] = useState("");
  const [asset, setAsset] = useState(() => new URLSearchParams(location.search).get("actif")?.toUpperCase() ?? "");
  const [q, setQ] = useState("");
  const [search, setSearch] = useState("");
  const [pages, setPages] = useState(1);
  const [more, setMore] = useState<Op[]>([]);
  const [loadingMore, setLoadingMore] = useState(false);

  // Typing settles before it searches.
  useEffect(() => {
    const t = setTimeout(() => setSearch(q.trim()), 250);
    return () => clearTimeout(t);
  }, [q]);

  const accounts = useLoad("accounts" + rev, (s) => get<Account[]>("accounts", s));
  const assets = useLoad("assets" + rev, (s) => get<string[]>("assets", s));
  const query = useMemo(() => {
    const p = new URLSearchParams({ limit: String(PAGE) });
    if (account) p.set("account", account);
    if (asset) p.set("asset", asset);
    if (search) p.set("q", search);
    return p.toString();
  }, [account, asset, search]);
  const list = useLoad(query + "#" + rev, (s) => get<Ops>("operations?" + query, s));
  useEffect(() => {
    setPages(1);
    setMore([]);
  }, [query, rev]);

  const loadMore = async () => {
    setLoadingMore(true);
    try {
      const r = await get<Ops>(`operations?${query}&offset=${pages * PAGE}`);
      setMore((m) => [...m, ...r.items]);
      setPages((p) => p + 1);
    } finally {
      setLoadingMore(false);
    }
  };

  const items = [...(list.data?.items ?? []), ...more];
  const total = list.data?.total ?? 0;
  const filtered = !!(account || asset || search);

  return (
    <div className="activity">
      <div className="page-head">
        <h1>Activité</h1>
        <button type="button" className="btn quiet" onClick={onImport}>
          <svg viewBox="0 0 16 16" aria-hidden>
            <path d="M8 10V2m0 0 3.5 3.5M8 2 4.5 5.5M2.5 10.5v2a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-2" />
          </svg>
          Importer
        </button>
      </div>

      <div className="filters">
        <label className="search">
          <svg viewBox="0 0 16 16" aria-hidden>
            <circle cx="7" cy="7" r="4.5" />
            <path d="M10.5 10.5 14 14" />
          </svg>
          <span className="sr">Rechercher</span>
          <input type="search" placeholder="Rechercher" value={q} onChange={(e) => setQ(e.target.value)} />
        </label>
        <label className={account ? "pick on" : "pick"}>
          <span className="sr">Compte</span>
          <select value={account} onChange={(e) => setAccount(e.target.value)}>
            <option value="">Tous les comptes</option>
            {accounts.data?.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </label>
        <label className={asset ? "pick on" : "pick"}>
          <span className="sr">Actif</span>
          <select value={asset} onChange={(e) => setAsset(e.target.value)}>
            <option value="">Tous les actifs</option>
            {assets.data?.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
      </div>

      {list.error && !list.data ? (
        <Failed error={list.error} retry={list.reload} />
      ) : !list.data ? (
        <SkelRows rows={8} />
      ) : items.length === 0 ? (
        <p className="empty">{filtered ? "Aucune opération ne correspond." : "Aucune opération pour l'instant."}</p>
      ) : (
        <div className={list.loading ? "stale" : ""} aria-busy={list.loading}>
          <p className="count muted">
            {total} opération{total > 1 ? "s" : ""}
          </p>
          <OpList items={items} onOpen={onOpen} />
          {items.length < total && (
            <button type="button" className="btn more" onClick={loadMore} disabled={loadingMore}>
              {loadingMore ? "Chargement…" : `Voir plus (${total - items.length})`}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
