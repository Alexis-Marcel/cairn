// The first screen, before any operation: three ways to start.

import { Mark } from "../components/Mark";

export function Welcome({ onAdd, onImport, onSync }: { onAdd: () => void; onImport: () => void; onSync: () => void }) {
  const ways = [
    {
      title: "Ajouter une opération",
      text: "Un achat, une vente, un dividende.",
      on: onAdd,
      icon: <path d="M12 5v14M5 12h14" />,
    },
    {
      title: "Importer un CSV",
      text: "Le modèle de Cairn ou l'export de Fundarr.",
      on: onImport,
      icon: <path d="M12 15V4m0 0 4 4m-4-4-4 4M5 15v3a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-3" />,
    },
    {
      title: "Synchroniser avec Fundarr",
      text: "Tes plateformes, à jour toutes seules.",
      on: onSync,
      icon: <path d="M19 8a7.5 7.5 0 0 0-13.4-2M5 16a7.5 7.5 0 0 0 13.4 2M19 3v5h-5M5 21v-5h5" />,
    },
  ];
  return (
    <div className="welcome">
      <Mark />
      <h1>Pose la première pierre</h1>
      <p className="muted">Ce que valent tes placements, et ce qu'ils t'ont rapporté.</p>
      <ul className="ways">
        {ways.map((w, n) => (
          <li key={w.title}>
            <button type="button" className={n === 0 ? "way first-way" : "way"} onClick={w.on}>
              <svg viewBox="0 0 24 24" aria-hidden>
                {w.icon}
              </svg>
              <span>
                <span className="way-title">{w.title}</span>
                <span className="way-text">{w.text}</span>
              </span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
