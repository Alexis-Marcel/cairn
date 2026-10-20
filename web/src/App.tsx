// The pages, a header and, on phones, a tab bar. Routing is the address
// bar, nothing more. Before any operation, Accueil offers three ways to start.

import { useCallback, useEffect, useState } from "react";
import { get, SCOPES, useLoad, type Op, type Scope, type Status } from "./api";
import { Mark } from "./components/Mark";
import { OpDialog } from "./components/OpDialog";
import { Failed, Loading } from "./components/State";
import { ago } from "./format";
import { remember, remembered } from "./lib/remember";
import { Activity } from "./pages/Activity";
import { Asset } from "./pages/Asset";
import { Assets } from "./pages/Assets";
import { Home } from "./pages/Home";
import { Import } from "./pages/Import";
import { Settings } from "./pages/Settings";
import { Taxes } from "./pages/Taxes";
import { Welcome } from "./pages/Welcome";

type Route = { page: "home" | "assets" | "asset" | "activity" | "taxes" | "settings" | "import"; symbol?: string };

function route(): Route {
  const p = decodeURIComponent(location.pathname.replace(/\/+$/, "")) || "/";
  const m = p.match(/^\/actifs\/([^/]+)$/);
  if (m) return { page: "asset", symbol: m[1].toUpperCase() };
  switch (p) {
    case "/actifs":
      return { page: "assets" };
    case "/activite":
    case "/operations":
      return { page: "activity" };
    case "/impots":
      return { page: "taxes" };
    case "/reglages":
      return { page: "settings" };
    case "/importer":
      return { page: "import" };
  }
  return { page: "home" };
}

const titles: Record<Route["page"], string> = {
  home: "Cairn",
  assets: "Actifs – Cairn",
  asset: "Cairn",
  activity: "Activité – Cairn",
  taxes: "Impôts – Cairn",
  settings: "Réglages – Cairn",
  import: "Importer – Cairn",
};

const TABS: { path: string; label: string; page: Route["page"][]; icon: React.ReactNode }[] = [
  {
    path: "/",
    label: "Accueil",
    page: ["home"],
    icon: <path d="M3.5 17.5 8.5 11l3.5 3.5 4.5-7 4 5" />,
  },
  {
    path: "/actifs",
    label: "Actifs",
    page: ["assets", "asset"],
    icon: (
      <>
        <ellipse cx="12" cy="18" rx="8" ry="2.6" />
        <ellipse cx="11" cy="12.6" rx="5.6" ry="2.3" />
        <ellipse cx="12.6" cy="7.6" rx="3.4" ry="1.9" />
      </>
    ),
  },
  {
    path: "/activite",
    label: "Activité",
    page: ["activity"],
    icon: <path d="M4 7h16M4 12h16M4 17h10" />,
  },
];

export function App() {
  const [r, setR] = useState<Route>(route);
  // Bumped after every change: the pages load again.
  const [rev, setRev] = useState(0);
  const [dialog, setDialog] = useState<{ op?: Op } | null>(null);
  const status = useLoad("status" + rev, (s) => get<Status>("status", s));
  const [picked, setPicked] = useState<Scope>(() => remembered("scope", "all", SCOPES));
  const pickScope = useCallback((v: Scope) => {
    setPicked(v);
    remember("scope", v);
  }, []);

  // N adds an operation, unless something is being typed or a dialog is open.
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() !== "n" || e.metaKey || e.ctrlKey || e.altKey || e.repeat) return;
      const el = e.target as HTMLElement | null;
      if (el?.closest("input, textarea, select, [contenteditable], dialog")) return;
      e.preventDefault();
      setDialog((d) => d ?? {});
    };
    addEventListener("keydown", on);
    return () => removeEventListener("keydown", on);
  }, []);

  useEffect(() => {
    const on = () => setR(route());
    addEventListener("popstate", on);
    return () => removeEventListener("popstate", on);
  }, []);
  useEffect(() => {
    // An asset's page names itself once it knows its name.
    if (r.page !== "asset") document.title = titles[r.page];
  }, [r]);

  const go = useCallback((path: string) => {
    if (path !== location.pathname + location.search) history.pushState(null, "", path);
    setR(route());
    scrollTo(0, 0);
  }, []);
  const link = (path: string) => (e: React.MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    go(path);
  };
  const changed = useCallback(() => setRev((n) => n + 1), []);
  const add = () => setDialog({});

  const st = status.data;
  if (!st) return status.error ? <Failed error={status.error} retry={status.reload} /> : <Loading />;

  const empty = st.activities === 0;
  const sync = st.sync;
  // With one class only, there is nothing to switch: everything is shown.
  const scope: Scope = st.scopes ? picked : "all";
  const scoping = st.scopes ? { scope, onScope: pickScope } : { scope };

  let page: React.ReactNode;
  switch (r.page) {
    case "activity":
      page = <Activity {...scoping} rev={rev} onOpen={(op) => setDialog({ op })} onImport={() => go("/importer")} />;
      break;
    case "assets":
      page = <Assets {...scoping} rev={rev} go={go} />;
      break;
    case "asset":
      page = <Asset key={r.symbol} symbol={r.symbol!} rev={rev} go={go} onOpen={(op) => setDialog({ op })} />;
      break;
    case "taxes":
      page = <Taxes rev={rev} />;
      break;
    case "settings":
      page = <Settings
          sync={sync}
          rev={rev}
          onChange={changed}
          onImport={() => go("/importer")}
          onFirstLink={empty ? () => go("/") : undefined}
        />;
      break;
    case "import":
      page = <Import onDone={changed} onHome={() => go("/")} />;
      break;
    default:
      page = empty ? (
        <Welcome onAdd={add} onImport={() => go("/importer")} onSync={() => go("/reglages#fundarr")} />
      ) : (
        <Home {...scoping} rev={rev} go={go} />
      );
  }

  const gear = (
    <a href="/reglages" onClick={link("/reglages")} className="icon-link" aria-label="Réglages" aria-current={r.page === "settings" ? "page" : undefined}>
      <svg viewBox="0 0 24 24" aria-hidden>
        <circle cx="12" cy="12" r="3" />
        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
      </svg>
    </a>
  );

  return (
    <>
      <a href="#main" className="skip">
        Aller au contenu
      </a>
      <header className="top">
        <a href="/" className="brand" onClick={link("/")} aria-label="Cairn, accueil">
          <Mark />
          <span className="brand-name">Cairn</span>
        </a>
        <nav aria-label="Pages" className="tabs">
          {TABS.map((t) => (
            <a key={t.path} href={t.path} onClick={link(t.path)} aria-current={t.page.includes(r.page) ? "page" : undefined}>
              {t.label}
            </a>
          ))}
        </nav>
        <div className="top-end">
          {gear}
          <button type="button" className="btn primary add" onClick={add} title="Ajouter (N)" aria-keyshortcuts="N">
            <svg viewBox="0 0 16 16" aria-hidden>
              <path d="M8 3v10M3 8h10" />
            </svg>
            Ajouter
          </button>
        </div>
      </header>
      <main id="main">{page}</main>
      <footer className="site-foot">
        <a href="/impots" onClick={link("/impots")} aria-current={r.page === "taxes" ? "page" : undefined}>
          Impôts
        </a>
        <a href="/importer" onClick={link("/importer")} aria-current={r.page === "import" ? "page" : undefined}>
          Importer
        </a>
        <a href="/reglages" onClick={link("/reglages")} aria-current={r.page === "settings" ? "page" : undefined}>
          Réglages
        </a>
        {sync.linked && (
          <a href="/reglages" onClick={link("/reglages")} className={sync.lastError ? "sync-state down" : "sync-state"} title={sync.lastError || sync.url}>
            <span className="dot" aria-hidden />
            {sync.lastError ? "Fundarr en erreur" : sync.lastSync ? `Fundarr · ${ago(sync.lastSync)}` : "Fundarr"}
          </a>
        )}
      </footer>
      <nav className="tabbar" aria-label="Pages">
        {TABS.map((t) => (
          <a key={t.path} href={t.path} onClick={link(t.path)} aria-current={t.page.includes(r.page) ? "page" : undefined}>
            <svg viewBox="0 0 24 24" aria-hidden>
              {t.icon}
            </svg>
            {t.label}
          </a>
        ))}
        <button type="button" onClick={add} className="tab-add">
          <span className="tab-add-dot" aria-hidden>
            <svg viewBox="0 0 24 24">
              <path d="M12 6v12M6 12h12" />
            </svg>
          </span>
          Ajouter
        </button>
      </nav>
      {dialog && (
        <OpDialog
          op={dialog.op}
          asset={!dialog.op && r.page === "asset" ? r.symbol : undefined}
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
