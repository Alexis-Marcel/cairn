// The pages and a header. Routing is the address bar, nothing more. Before
// any operation, Accueil offers three ways to start.

import { useCallback, useEffect, useState } from "react";
import { get, useLoad, type Op, type Status } from "./api";
import { Mark } from "./components/Mark";
import { OpDialog } from "./components/OpDialog";
import { Failed, Loading } from "./components/State";
import { ago } from "./format";
import { Home } from "./pages/Home";
import { Import } from "./pages/Import";
import { Operations } from "./pages/Operations";
import { Settings } from "./pages/Settings";
import { Taxes } from "./pages/Taxes";
import { Welcome } from "./pages/Welcome";

type Path = "/" | "/operations" | "/impots" | "/reglages" | "/importer";
const titles: Record<Path, string> = {
  "/": "Cairn",
  "/operations": "Opérations – Cairn",
  "/impots": "Impôts – Cairn",
  "/reglages": "Réglages – Cairn",
  "/importer": "Importer – Cairn",
};
const nav: { path: Path; label: string }[] = [
  { path: "/", label: "Accueil" },
  { path: "/operations", label: "Opérations" },
  { path: "/impots", label: "Impôts" },
];

const current = (): Path => {
  const p = location.pathname.replace(/\/+$/, "") || "/";
  return p in titles ? (p as Path) : "/";
};

export function App() {
  const [path, setPath] = useState<Path>(current);
  // Bumped after every change: the pages load again.
  const [rev, setRev] = useState(0);
  const [dialog, setDialog] = useState<{ op?: Op } | null>(null);
  const status = useLoad("status" + rev, (s) => get<Status>("status", s));

  useEffect(() => {
    const on = () => setPath(current());
    addEventListener("popstate", on);
    return () => removeEventListener("popstate", on);
  }, []);
  useEffect(() => {
    document.title = titles[path];
  }, [path]);

  const to = useCallback(
    (p: Path, hash = "") => {
      if (p !== path || hash) {
        history.pushState(null, "", p + hash);
        setPath(p);
      }
      scrollTo(0, 0);
    },
    [path],
  );
  const go = (p: Path) => (e: React.MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    to(p);
  };
  const changed = useCallback(() => setRev((r) => r + 1), []);
  const add = () => setDialog({});

  const st = status.data;
  if (!st)
    return status.error ? <Failed error={status.error} retry={status.reload} /> : <Loading />;

  const empty = st.activities === 0;
  const sync = st.sync;

  let page: React.ReactNode;
  switch (path) {
    case "/operations":
      page = <Operations rev={rev} onOpen={(op) => setDialog({ op })} onImport={() => to("/importer")} />;
      break;
    case "/impots":
      page = <Taxes rev={rev} />;
      break;
    case "/reglages":
      page = <Settings sync={sync} rev={rev} onChange={changed} onImport={() => to("/importer")} />;
      break;
    case "/importer":
      page = <Import onDone={changed} onHome={() => to("/")} />;
      break;
    default:
      page = empty ? (
        <Welcome onAdd={add} onImport={() => to("/importer")} onSync={() => to("/reglages", "#fundarr")} />
      ) : (
        <Home rev={rev} />
      );
  }

  return (
    <>
      <header className="top">
        <a href="/" className="brand" onClick={go("/")} aria-label="Cairn, accueil">
          <Mark />
          <span className="brand-name">Cairn</span>
        </a>
        <nav aria-label="Pages">
          {nav.map((n) => (
            <a key={n.path} href={n.path} onClick={go(n.path)} aria-current={n.path === path ? "page" : undefined}>
              {n.label}
            </a>
          ))}
        </nav>
        <button type="button" className="btn primary add" onClick={add} aria-label="Ajouter une opération">
          <svg viewBox="0 0 16 16" aria-hidden>
            <path d="M8 3v10M3 8h10" />
          </svg>
          <span className="add-label">Ajouter</span>
        </button>
      </header>
      <main>{page}</main>
      <footer className="site-foot">
        <a href="/reglages" onClick={go("/reglages")} aria-current={path === "/reglages" ? "page" : undefined}>
          Réglages
        </a>
        {sync.linked && (
          <a href="/reglages" onClick={go("/reglages")} className={sync.lastError ? "sync-state down" : "sync-state"} title={sync.lastError || sync.url}>
            <span className="dot" aria-hidden />
            {sync.lastError ? "Fundarr en erreur" : sync.lastSync ? `Fundarr · ${ago(sync.lastSync)}` : "Fundarr"}
          </a>
        )}
      </footer>
      {dialog && (
        <OpDialog
          op={dialog.op}
          onClose={() => setDialog(null)}
          onSaved={() => {
            setDialog(null);
            changed();
          }}
        />
      )}
    </>
  );
}
