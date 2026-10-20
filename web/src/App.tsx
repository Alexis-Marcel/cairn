// Three pages and a header. Routing is the address bar, nothing more. Until
// Cairn is linked to a Fundarr, there is only the page that links it.

import { useEffect, useState } from "react";
import { get, type Setup } from "./api";
import { LinkForm } from "./components/LinkForm";
import { Mark } from "./components/Mark";
import { Failed, Loading } from "./components/State";
import { Home } from "./pages/Home";
import { Settings } from "./pages/Settings";
import { Taxes } from "./pages/Taxes";

type Path = "/" | "/impots" | "/reglages";
const titles: Record<Path, string> = { "/": "Cairn", "/impots": "Impôts – Cairn", "/reglages": "Réglages – Cairn" };

const current = (): Path => {
  const p = location.pathname.replace(/\/+$/, "") || "/";
  return p in titles ? (p as Path) : "/";
};

export function App() {
  const [path, setPath] = useState<Path>(current);
  const [setup, setSetup] = useState<Setup>();
  const [error, setError] = useState("");
  const [tick, setTick] = useState(0);

  useEffect(() => {
    const on = () => setPath(current());
    addEventListener("popstate", on);
    return () => removeEventListener("popstate", on);
  }, []);
  useEffect(() => {
    const ctl = new AbortController();
    get<Setup>("setup", ctl.signal).then(setSetup, (e) => {
      if (!ctl.signal.aborted) setError(e instanceof Error ? e.message : String(e));
    });
    return () => ctl.abort();
  }, [tick]);
  const linked = setup?.configured;
  useEffect(() => {
    document.title = linked === false ? "Relier – Cairn" : titles[path];
  }, [path, linked]);

  const to = (p: Path) => {
    if (p !== path) {
      history.pushState(null, "", p);
      setPath(p);
    }
    scrollTo(0, 0);
  };
  const go = (p: Path) => (e: React.MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    to(p);
  };
  // Linked (again): the dashboard. Unlinked: the first-run page.
  const changed = (s: Setup) => {
    setSetup(s);
    to("/");
  };

  if (!setup)
    return error ? (
      <Failed
        error={error}
        retry={() => {
          setError("");
          setTick((t) => t + 1);
        }}
      />
    ) : (
      <Loading />
    );

  if (!setup.configured)
    return (
      <main className="first">
        <Mark />
        <h1>Relier Cairn à Fundarr</h1>
        <LinkForm onLinked={changed} />
      </main>
    );

  return (
    <>
      <header className="top">
        <a href="/" className="brand" onClick={go("/")}>
          <Mark />
          Cairn
        </a>
        <nav aria-label="Pages">
          {(["/", "/impots"] as Path[]).map((p) => (
            <a key={p} href={p} onClick={go(p)} aria-current={p === path ? "page" : undefined}>
              {p === "/" ? "Accueil" : "Impôts"}
            </a>
          ))}
        </nav>
      </header>
      <main>
        {path === "/impots" ? <Taxes /> : path === "/reglages" ? <Settings setup={setup} onChange={changed} /> : <Home />}
      </main>
      {path !== "/reglages" && (
        <footer className="site-foot">
          <a href="/reglages" onClick={go("/reglages")}>
            Réglages
          </a>
        </footer>
      )}
    </>
  );
}
