// The address and token of a Fundarr. Cairn tries them before keeping them,
// then copies the history once.

import { useEffect, useId, useRef, useState } from "react";
import { get, message, send, type SyncState } from "../api";

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
  const [found, setFound] = useState(false);
  const tokenRef = useRef<HTMLInputElement>(null);
  const typed = useRef(false);

  // A Fundarr on the same server fills in its own address.
  useEffect(() => {
    if (initial) return;
    const ctl = new AbortController();
    get<{ url: string }>("sync/find", ctl.signal)
      .then((r) => {
        if (!r.url || typed.current) return;
        setUrl(r.url);
        setFound(true);
        tokenRef.current?.focus();
      })
      .catch(() => {});
    return () => ctl.abort();
  }, [initial]);

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
          onChange={(e) => {
            typed.current = true;
            setUrl(e.target.value);
            setFound(false);
          }}
        />
        {found && <small className="muted">Fundarr trouvé sur ce serveur.</small>}
      </label>
      <div className="field">
        <label htmlFor={id + "-token"}>Jeton</label>
        <input
          id={id + "-token"}
          ref={tokenRef}
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
