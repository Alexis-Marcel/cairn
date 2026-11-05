// Réglages: the link to Fundarr (optional), assistants' tokens, and the accounts.

import { useEffect, useId, useState } from "react";
import { get, message, send, useLoad, type Account, type McpState, type SyncState } from "../api";
import { LinkForm } from "../components/LinkForm";
import { ago } from "../format";

export function Settings({
  sync,
  rev,
  onChange,
  onImport,
  onFirstLink,
}: {
  sync: SyncState;
  rev: number;
  onChange: () => void;
  onImport: () => void;
  /** A first link, with nothing in Cairn yet: the portfolio is what to see next. */
  onFirstLink?: () => void;
}) {
  return (
    <div className="settings">
      <h1>Réglages</h1>
      <Fundarr sync={sync} onChange={onChange} onFirstLink={onFirstLink} />
      <Assistants />
      <Accounts rev={rev} onChange={onChange} />
      <section aria-labelledby="h-data">
        <h2 id="h-data">Fichiers</h2>
        <div className="actions">
          <button type="button" className="btn" onClick={onImport}>
            Importer un CSV
          </button>
          <a className="btn quiet" href="/api/import/modele.csv" download>
            Modèle CSV
          </a>
        </div>
      </section>
    </div>
  );
}

function Fundarr({ sync, onChange, onFirstLink }: { sync: SyncState; onChange: () => void; onFirstLink?: () => void }) {
  const [mode, setMode] = useState<"view" | "change" | "unlink">(() => (location.hash === "#fundarr" && !sync.linked ? "change" : "view"));
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const act = async (f: () => Promise<unknown>) => {
    setBusy(true);
    setError("");
    try {
      await f();
      setMode("view");
      onChange();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  const err = error && (
    <p className="form-error" role="alert">
      {error}
    </p>
  );

  return (
    <section aria-labelledby="h-fundarr" id="fundarr">
      <h2 id="h-fundarr">Fundarr</h2>
      {!sync.linked ? (
        mode === "change" ? (
          <LinkForm
            onLinked={() => {
              setMode("view");
              onChange();
              onFirstLink?.();
            }}
            onCancel={() => setMode("view")}
          />
        ) : (
          <>
            <p className="muted note">Copie toute seule l'historique de tes plateformes. Facultatif.</p>
            <div className="actions">
              <button type="button" className="btn" onClick={() => setMode("change")}>
                Synchroniser avec Fundarr
              </button>
            </div>
          </>
        )
      ) : (
        <>
          <div className="link-state">
            <span className={sync.lastError ? "link-dot down" : busy || sync.running ? "link-dot busy" : "link-dot"} aria-hidden />
            <div className="link-text">
              <p className={sync.lastError ? "down" : ""} aria-live="polite">
                {busy || sync.running
                  ? "Synchronisation…"
                  : sync.lastError
                    ? sync.lastError
                    : sync.lastSync
                      ? `Synchronisé ${ago(sync.lastSync)}`
                      : "Pas encore synchronisé"}
              </p>
              <p className="muted link-url">{sync.url}</p>
            </div>
            {mode === "view" && (
              <button type="button" className="btn" onClick={() => act(() => send("POST", "sync/now"))} disabled={busy || sync.running}>
                Synchroniser
              </button>
            )}
          </div>
          {mode === "change" ? (
            <LinkForm
              url={sync.url}
              onLinked={() => {
                setMode("view");
                onChange();
              }}
              onCancel={() => setMode("view")}
            />
          ) : mode === "unlink" ? (
            <div className="confirm">
              <p>Garder les opérations copiées de Fundarr ?</p>
              {err}
              <div className="actions">
                <button type="button" className="btn" onClick={() => act(() => send("DELETE", "sync", { keep: true }))} disabled={busy}>
                  Délier et garder
                </button>
                <button type="button" className="btn danger" onClick={() => act(() => send("DELETE", "sync", { keep: false }))} disabled={busy}>
                  Délier et supprimer
                </button>
                <button type="button" className="btn quiet" onClick={() => setMode("view")} disabled={busy}>
                  Annuler
                </button>
              </div>
            </div>
          ) : (
            <>
              {err}
              {sync.fromEnv ? (
                <p className="muted small-note">Lien fixé par l'environnement.</p>
              ) : (
                <div className="actions link-actions">
                  <button type="button" className="btn quiet small" onClick={() => setMode("change")}>
                    Changer
                  </button>
                  <button type="button" className="btn quiet small" onClick={() => setMode("unlink")}>
                    Délier
                  </button>
                </div>
              )}
              <Notify sync={sync} onChange={onChange} />
            </>
          )}
        </>
      )}
    </section>
  );
}

/** Being told by Fundarr instead of asking every 15 minutes. */
function Notify({ sync, onChange }: { sync: SyncState; onChange: () => void }) {
  const id = useId();
  const [secret, setSecret] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const url = `${location.origin}/api/fundarr/notify`;

  const save = async (value: string) => {
    setBusy(true);
    setError("");
    try {
      await send("PUT", "sync/secret", { secret: value });
      setSecret("");
      onChange();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <details className="notify">
      <summary>
        Être prévenu par Fundarr
        {sync.notify && <span className="badge fundarr">Actif</span>}
      </summary>
      <div className="notify-body">
        <p className="muted note">Sinon, Cairn demande toutes les 15 minutes.</p>
        <ol>
          <li>
            Dans Fundarr, Apps → Cairn → Notify, colle cette adresse :
            <Copy value={url} />
          </li>
          <li>
            <label htmlFor={id}>Puis colle ici le secret qu'il affiche :</label>
            <form
              className="with-action"
              onSubmit={(e) => {
                e.preventDefault();
                if (secret.trim()) save(secret);
              }}
            >
              <input
                id={id}
                type="password"
                placeholder={sync.notify ? "Secret enregistré" : "fns_…"}
                autoComplete="off"
                spellCheck={false}
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
              />
              <button type="submit" className="btn" disabled={busy || !secret.trim()}>
                Enregistrer
              </button>
            </form>
          </li>
        </ol>
        <p className="muted small-note">
          Fundarr doit pouvoir joindre cette adresse : c'est la seule à laisser passer sans connexion sur ton proxy.
        </p>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        {sync.notify && (
          <button type="button" className="btn quiet small" onClick={() => save("")} disabled={busy}>
            Arrêter les notifications
          </button>
        )}
      </div>
    </details>
  );
}

/** A value to copy, with its button. */
function Copy({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => setCopied(false), 1500);
    return () => clearTimeout(t);
  }, [copied]);
  return (
    <span className="copy">
      <code>{value}</code>
      <button
        type="button"
        className="btn quiet small"
        onClick={() => navigator.clipboard?.writeText(value).then(() => setCopied(true), () => {})}
      >
        {copied ? "Copié" : "Copier"}
      </button>
    </span>
  );
}

/** The tokens assistants read Cairn with, over MCP: one each. */
function Assistants() {
  const [rev, setRev] = useState(0);
  const state = useLoad("mcp" + rev, (s) => get<McpState>("mcp", s));
  const [name, setName] = useState("");
  const [fresh, setFresh] = useState<{ id: number; name: string; token: string } | null>(null);
  const [revoking, setRevoking] = useState<number | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const act = async (f: () => Promise<unknown>) => {
    setBusy(true);
    setError("");
    try {
      await f();
      setRevoking(null);
      setRev((r) => r + 1);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };
  const create = () =>
    act(async () => {
      setFresh(await send<{ id: number; name: string; token: string }>("POST", "mcp/tokens", { name }));
      setName("");
    });
  const revoke = (t: number) =>
    act(async () => {
      await send("DELETE", `mcp/tokens/${t}`);
      if (fresh?.id === t) setFresh(null);
    });

  const mcp = state.data;
  if (!mcp || mcp.readOnly) return null;
  return (
    <section aria-labelledby="h-assistants">
      <h2 id="h-assistants">Assistants</h2>
      <p className="muted note">
        Donne à un assistant IA un accès en lecture seule à ton portefeuille, par MCP. Ce qu'il lit peut partir chez le fournisseur de son modèle.
      </p>
      {(mcp.fromEnv || mcp.tokens.length > 0) && (
        <>
          <p className="muted small-note">Adresse du serveur MCP :</p>
          <Copy value={`${location.origin}/mcp`} />
        </>
      )}
      {mcp.fromEnv ? (
        <p className="muted small-note">Jeton fixé par l'environnement.</p>
      ) : (
        <>
          {mcp.tokens.length > 0 && (
            <ul className="accounts">
              {mcp.tokens.map((t) => (
                <li key={t.id}>
                  <span className="acc-name">{t.name}</span>
                  <span className="muted num">{t.createdAt ? `Créé ${ago(t.createdAt)}` : ""}</span>
                  <span className="acc-actions">
                    {revoking === t.id ? (
                      <>
                        <button type="button" className="btn danger small" onClick={() => revoke(t.id)} disabled={busy}>
                          Confirmer
                        </button>
                        <button type="button" className="btn quiet small" onClick={() => setRevoking(null)} disabled={busy}>
                          Annuler
                        </button>
                      </>
                    ) : (
                      <button type="button" className="btn quiet small" onClick={() => setRevoking(t.id)}>
                        Révoquer
                      </button>
                    )}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {fresh && (
            <>
              <p className="small-note">Jeton pour {fresh.name}. Copie-le maintenant : il ne sera plus affiché.</p>
              <Copy value={fresh.token} />
            </>
          )}
          <form
            className="with-action token-form"
            onSubmit={(e) => {
              e.preventDefault();
              if (name.trim()) create();
            }}
          >
            <input
              type="text"
              aria-label="Nom de l'assistant"
              placeholder="Nom de l'assistant"
              maxLength={60}
              autoComplete="off"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <button type="submit" className="btn" disabled={busy || !name.trim()}>
              Créer un jeton
            </button>
          </form>
        </>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

function Accounts({ rev, onChange }: { rev: number; onChange: () => void }) {
  const list = useLoad("accounts" + rev, (s) => get<Account[]>("accounts", s));
  const [editing, setEditing] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  const act = async (f: () => Promise<unknown>) => {
    setError("");
    try {
      await f();
      setEditing(null);
      onChange();
    } catch (e) {
      setError(message(e));
    }
  };

  if (!list.data || list.data.length === 0) return null;
  // Read-only accounts are told apart only when there are others.
  const mixed = new Set(list.data.map((a) => a.origin)).size > 1;
  return (
    <section aria-labelledby="h-accounts">
      <h2 id="h-accounts">Comptes</h2>
      <ul className="accounts">
        {list.data.map((a) => (
          <li key={a.id}>
            {editing === a.id ? (
              <form
                className="with-action"
                onSubmit={(e) => {
                  e.preventDefault();
                  act(() => send("PATCH", `accounts/${a.id}`, { name }));
                }}
              >
                <input type="text" aria-label="Nom du compte" maxLength={60} autoFocus value={name} onChange={(e) => setName(e.target.value)} />
                <button type="submit" className="btn small" disabled={!name.trim()}>
                  OK
                </button>
                <button type="button" className="btn quiet small" onClick={() => setEditing(null)}>
                  Annuler
                </button>
              </form>
            ) : (
              <>
                <span className="acc-name">
                  {a.name}
                  {mixed && a.origin === "fundarr" && <span className="badge fundarr">Fundarr</span>}
                </span>
                <span className="muted num">{a.count === 0 ? "Aucune opération" : `${a.count} opération${a.count > 1 ? "s" : ""}`}</span>
                {a.origin === "manual" && (
                  <span className="acc-actions">
                    <button
                      type="button"
                      className="btn quiet small"
                      onClick={() => {
                        setEditing(a.id);
                        setName(a.name);
                      }}
                    >
                      Renommer
                    </button>
                    {a.count === 0 && (
                      <button type="button" className="btn quiet small" onClick={() => act(() => send("DELETE", `accounts/${a.id}`))}>
                        Supprimer
                      </button>
                    )}
                  </span>
                )}
              </>
            )}
          </li>
        ))}
      </ul>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}
