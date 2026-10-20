// Réglages: the link to Fundarr (optional), and the accounts.

import { useEffect, useId, useState } from "react";
import { get, message, send, useLoad, type Account, type SyncState } from "../api";
import { LinkForm } from "../components/LinkForm";
import { ago } from "../format";

export function Settings({ sync, rev, onChange, onImport }: { sync: SyncState; rev: number; onChange: () => void; onImport: () => void }) {
  return (
    <div className="settings">
      <h1>Réglages</h1>
      <Fundarr sync={sync} onChange={onChange} />
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

function Fundarr({ sync, onChange }: { sync: SyncState; onChange: () => void }) {
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
          <dl className="linked">
            <dt>Adresse</dt>
            <dd>{sync.url}</dd>
          </dl>
          <p className={sync.lastError ? "sync-line down" : "sync-line muted"}>
            {busy || sync.running
              ? "Synchronisation…"
              : sync.lastError
                ? sync.lastError
                : sync.lastSync
                  ? `Synchronisé ${ago(sync.lastSync)}`
                  : "Pas encore synchronisé"}
          </p>
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
              <div className="actions">
                <button type="button" className="btn" onClick={() => act(() => send("POST", "sync/now"))} disabled={busy || sync.running}>
                  Synchroniser
                </button>
                {sync.fromEnv ? (
                  <span className="muted note">Fixé par l'environnement.</span>
                ) : (
                  <>
                    <button type="button" className="btn quiet" onClick={() => setMode("change")}>
                      Changer
                    </button>
                    <button type="button" className="btn quiet" onClick={() => setMode("unlink")}>
                      Délier
                    </button>
                  </>
                )}
              </div>
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
  const [copied, setCopied] = useState(false);
  const url = `${location.origin}/api/fundarr/notify`;
  useEffect(() => {
    if (!copied) return;
    const t = setTimeout(() => setCopied(false), 1500);
    return () => clearTimeout(t);
  }, [copied]);

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
            <span className="copy">
              <code>{url}</code>
              <button
                type="button"
                className="btn quiet small"
                onClick={() => navigator.clipboard?.writeText(url).then(() => setCopied(true), () => {})}
              >
                {copied ? "Copié" : "Copier"}
              </button>
            </span>
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
                  {a.origin === "fundarr" && <span className="badge fundarr">Fundarr</span>}
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
