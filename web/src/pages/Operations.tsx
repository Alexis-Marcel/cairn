// Opérations: every activity, newest first, whatever its origin. Filter by
// account or coin, search, open one to change it.

import { useEffect, useMemo, useState } from "react";
import { get, useLoad, type Account, type Op, type Ops } from "../api";
import { Coin } from "../components/Coin";
import { Failed, Loading } from "../components/State";
import { date, eur, num } from "../format";
import { KIND, ORIGIN, detail, moves } from "../ops";

const PAGE = 50;

export function Operations({ rev, onOpen, onImport }: { rev: number; onOpen: (op: Op) => void; onImport: () => void }) {
  const [account, setAccount] = useState("");
  const [asset, setAsset] = useState("");
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
    <div className="ops">
      <div className="page-head">
        <h1>Opérations</h1>
        <div className="actions">
          <button type="button" className="btn" onClick={onImport}>
            Importer un CSV
          </button>
        </div>
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
        <label>
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
        <label>
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
        <Loading />
      ) : items.length === 0 ? (
        <p className="empty">{filtered ? "Aucune opération ne correspond." : "Aucune opération pour l'instant."}</p>
      ) : (
        <div className={list.loading ? "stale" : ""} aria-busy={list.loading}>
          <p className="count muted">
            {total} opération{total > 1 ? "s" : ""}
          </p>
          <table className="table num op-table">
            <thead>
              <tr>
                <th scope="col" className="wide">
                  Date
                </th>
                <th scope="col">Opération</th>
                <th scope="col" className="mid">
                  Compte
                </th>
                <th scope="col">Quantité</th>
                <th scope="col" className="wide">
                  Valeur
                </th>
                <th scope="col" className="mid">
                  <span className="sr">Origine</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((op) => {
                const m = moves(op);
                return (
                  <tr key={op.id} onClick={() => onOpen(op)} className="clickable">
                    <td className="wide date">{date(op.time)}</td>
                    <th scope="row">
                      <button type="button" className="row-btn" onClick={(e) => (e.stopPropagation(), onOpen(op))}>
                        <span className="asset">
                          <Coin symbol={m.symbol} />
                          <span>
                            <span className="sym">{KIND[op.kind]}</span>
                            <span className="sub op-sub">
                              <span className="op-acc">{detail(op)} · </span>
                              {date(op.time)}
                            </span>
                          </span>
                        </span>
                      </button>
                    </th>
                    <td className="mid muted left">{detail(op)}</td>
                    <td>
                      <span className={m.text.startsWith("+") ? "up" : ""}>{m.text}</span>
                      {m.other && <span className="sub always up">{m.other}</span>}
                      {op.value && <span className="sub op-val">{eur(num(op.value))}</span>}
                    </td>
                    <td className="wide muted">{op.value ? eur(num(op.value)) : "–"}</td>
                    <td className="mid">
                      <span className={`badge ${op.origin}`}>{ORIGIN[op.origin]}</span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
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
