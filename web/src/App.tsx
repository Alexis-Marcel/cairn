// Two pages and a header. Routing is the address bar, nothing more.

import { useEffect, useState } from "react";
import { Mark } from "./components/Mark";
import { Home } from "./pages/Home";
import { Taxes } from "./pages/Taxes";

const pages = { "/": Home, "/impots": Taxes } as const;
type Path = keyof typeof pages;

const current = (): Path => (location.pathname.replace(/\/+$/, "") || "/") as Path;

export function App() {
  const [path, setPath] = useState<Path>(current);
  useEffect(() => {
    const on = () => setPath(current());
    addEventListener("popstate", on);
    return () => removeEventListener("popstate", on);
  }, []);
  useEffect(() => {
    document.title = path === "/impots" ? "Impôts – Cairn" : "Cairn";
  }, [path]);

  const go = (to: Path) => (e: React.MouseEvent) => {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    if (to !== path) {
      history.pushState(null, "", to);
      setPath(to);
      scrollTo(0, 0);
    }
  };
  const Page = pages[path] ?? Home;

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
        <Page />
      </main>
    </>
  );
}
