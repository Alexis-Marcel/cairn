// The address and token of a Fundarr. Cairn tries them before keeping them,
// then copies the history once.

import { useId, useState } from "react";
import { message, send, type SyncState } from "../api";

export function LinkForm({
  url: initial = "",
  onLinked,
  onCancel,
}: {
  url?: string;
  onLinked: (s: SyncState) => void;
  onCancel?: () => void;
}) {
  const id = useId();
  const [url, setUrl] = useState(initial);
  const [token, setToken] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      onLinked(await send<SyncState>("POST", "sync", { url, token }));
    } catch (err) {
      setError(message(err));
      setBusy(false);
    }
  };

  return (
    <form className="link-form" onSubmit={submit} noValidate aria-busy={busy}>
      <label className="field">
        <span>Adresse de Fundarr</span>
        <input
          type="text"
          inputMode="url"
          name="url"
          placeholder="http://fundarr:9494"
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          required
          autoFocus={!initial}
          value={url}
          onChange={(e) => setUrl(e.target.value)}
        />
      </label>
      <div className="field">
        <label htmlFor={id + "-token"}>Jeton</label>
        <input
          id={id + "-token"}
          type="password"
          name="token"
          placeholder="fdr_…"
          autoComplete="off"
          spellCheck={false}
          required
          autoFocus={!!initial}
          aria-describedby={id + "-hint"}
          value={token}
          onChange={(e) => setToken(e.target.value)}
        />
        <small id={id + "-hint"} className="muted">
          Dans Fundarr : Apps → Add app → Cairn.
        </small>
      </div>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div className="actions">
        <button type="submit" className="btn primary" disabled={busy || !url.trim() || !token.trim()}>
          {busy ? "Synchronisation…" : "Synchroniser"}
        </button>
        {onCancel && (
          <button type="button" className="btn quiet" onClick={onCancel}>
            Annuler
          </button>
        )}
      </div>
    </form>
  );
}
