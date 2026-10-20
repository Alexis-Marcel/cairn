// Réglages: which Fundarr Cairn reads, to change it or let it go.

import { useState } from "react";
import { setLink, type Setup } from "../api";
import { LinkForm } from "../components/LinkForm";

export function Settings({ setup, onChange }: { setup: Setup; onChange: (s: Setup) => void }) {
  const [mode, setMode] = useState<"view" | "change" | "unlink">("view");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const unlink = async () => {
    setBusy(true);
    setError("");
    try {
      onChange(await setLink(null));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  };

  return (
    <div className="settings">
      <h1>Réglages</h1>
      <section>
        <dl className="linked">
          <dt>Fundarr</dt>
          <dd>{setup.url}</dd>
        </dl>
        {setup.fromEnv ? (
          <p className="muted note">Fixé par l'environnement.</p>
        ) : mode === "change" ? (
          <LinkForm url={setup.url} onLinked={onChange} onCancel={() => setMode("view")} />
        ) : mode === "unlink" ? (
          <div className="confirm">
            <p>Cairn ne lira plus ce Fundarr.</p>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <div className="actions">
              <button type="button" className="btn danger" onClick={unlink} disabled={busy}>
                Délier
              </button>
              <button type="button" className="btn quiet" onClick={() => setMode("view")} disabled={busy}>
                Annuler
              </button>
            </div>
          </div>
        ) : (
          <div className="actions">
            <button type="button" className="btn" onClick={() => setMode("change")}>
              Changer
            </button>
            <button type="button" className="btn quiet" onClick={() => setMode("unlink")}>
              Délier
            </button>
          </div>
        )}
      </section>
    </div>
  );
}
