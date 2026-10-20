// Importer: a CSV file (Cairn's template or Fundarr's export), checked row
// by row before anything is added. Importing the same file again adds
// nothing.

import { useState } from "react";
import { get, message, send, useLoad, type Account, type Preview } from "../api";
import { Coin } from "../components/Coin";
import { AccountPicker, type AccountChoice } from "../components/Pickers";
import { date } from "../format";
import { KIND, amount } from "../ops";

type Result = { added: number; skipped: number; errors: number };

export function Import({ onDone, onHome }: { onDone: () => void; onHome: () => void }) {
  const accounts = useLoad("accounts", (s) => get<Account[]>("accounts", s));
  const [file, setFile] = useState<{ name: string; csv: string } | null>(null);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [fromFile, setFromFile] = useState(true);
  const [target, setTarget] = useState<AccountChoice>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<Result | null>(null);
  const [drag, setDrag] = useState(false);

  const read = async (f: File | undefined) => {
    if (!f) return;
    setError("");
    setPreview(null);
    setResult(null);
    if (f.size > 20 << 20) {
      setError("Fichier trop lourd (20 Mo au plus)");
      return;
    }
    setBusy(true);
    try {
      const csv = await f.text();
      const p = await send<Preview>("POST", "import/preview", { csv });
      setFile({ name: f.name, csv });
      setPreview(p);
      setFromFile(p.fileAccounts);
      const list = accounts.data ?? [];
      setTarget(list.length ? { id: (list.find((a) => a.origin === "manual") ?? list[0]).id } : { name: "" });
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  const run = async () => {
    if (!file || !preview) return;
    setBusy(true);
    setError("");
    try {
      const body = fromFile
        ? { csv: file.csv, fromFile: true }
        : { csv: file.csv, ...(target.name !== undefined ? { newAccount: target.name } : { account: target.id }) };
      setResult(await send<Result>("POST", "import", body));
      onDone();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  const reset = () => {
    setFile(null);
    setPreview(null);
    setResult(null);
    setError("");
  };

  if (result)
    return (
      <div className="import">
        <h1>Importer</h1>
        <div className="done">
          <p className="done-big num">
            {result.added} opération{result.added > 1 ? "s" : ""} ajoutée{result.added > 1 ? "s" : ""}
          </p>
          {(result.skipped > 0 || result.errors > 0) && (
            <p className="muted">
              {[result.skipped > 0 && `${result.skipped} déjà là`, result.errors > 0 && `${result.errors} en erreur`].filter(Boolean).join(", ")}
            </p>
          )}
          <div className="actions">
            <button type="button" className="btn primary" onClick={onHome}>
              Voir l'accueil
            </button>
            <button type="button" className="btn quiet" onClick={reset}>
              Importer un autre fichier
            </button>
          </div>
        </div>
      </div>
    );

  const fresh = preview ? preview.valid : 0;
  const ready = preview && fresh > 0 && (fromFile || target.id || target.name?.trim());

  return (
    <div className="import">
      <h1>Importer</h1>
      {!preview ? (
        <>
          <label
            className={drag ? "drop on" : "drop"}
            onDragOver={(e) => {
              e.preventDefault();
              setDrag(true);
            }}
            onDragLeave={() => setDrag(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDrag(false);
              read(e.dataTransfer.files[0]);
            }}
          >
            <input type="file" accept=".csv,text/csv" className="sr" onChange={(e) => read(e.target.files?.[0])} disabled={busy} />
            <svg viewBox="0 0 24 24" aria-hidden>
              <path d="M12 15V4m0 0 4 4m-4-4-4 4M5 15v3a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-3" />
            </svg>
            <span className="drop-title">{busy ? "Lecture…" : "Choisir un fichier CSV"}</span>
            <span className="muted">ou le déposer ici</span>
          </label>
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          <div className="formats">
            <p>
              Le modèle de Cairn, ou l'export CSV de Fundarr.{" "}
              <a href="/api/import/modele.csv" download>
                Télécharger le modèle
              </a>
            </p>
            <details>
              <summary>Colonnes du modèle</summary>
              <dl className="cols">
                <div>
                  <dt>date</dt>
                  <dd>2024-03-01 ou 01/03/2024</dd>
                </div>
                <div>
                  <dt>type</dt>
                  <dd>achat, vente, echange, depot, retrait, recompense, frais</dd>
                </div>
                <div>
                  <dt>actif, quantite</dt>
                  <dd>BTC, 0,01</dd>
                </div>
                <div>
                  <dt>prix</dt>
                  <dd>achat et vente : le total en euros</dd>
                </div>
                <div>
                  <dt>frais</dt>
                  <dd>en euros, facultatif</dd>
                </div>
                <div>
                  <dt>recu, quantite_recue</dt>
                  <dd>échange : la crypto reçue</dd>
                </div>
                <div>
                  <dt>compte, note</dt>
                  <dd>facultatifs</dd>
                </div>
              </dl>
            </details>
          </div>
        </>
      ) : (
        <div className="preview">
          <div className="preview-head">
            <p>
              <strong>{file?.name}</strong>
              <span className="muted"> · {preview.format === "fundarr" ? "export Fundarr" : "modèle Cairn"}</span>
            </p>
            <button type="button" className="btn quiet small" onClick={reset}>
              Changer
            </button>
          </div>
          <dl className="tax-totals num">
            <div>
              <dt>À ajouter</dt>
              <dd>{preview.valid}</dd>
            </div>
            {preview.duplicates > 0 && (
              <div>
                <dt>Déjà là</dt>
                <dd>{preview.duplicates}</dd>
              </div>
            )}
            {preview.errors > 0 && (
              <div>
                <dt>En erreur</dt>
                <dd className="down">{preview.errors}</dd>
              </div>
            )}
          </dl>

          <div className="scroll preview-rows">
            <table className="table num">
              <thead>
                <tr>
                  <th scope="col">Ligne</th>
                  <th scope="col">Opération</th>
                  <th scope="col" className="mid">
                    Date
                  </th>
                  <th scope="col">Quantité</th>
                  <th scope="col" className="wide">
                    Compte
                  </th>
                </tr>
              </thead>
              <tbody>
                {preview.rows.map((r) =>
                  r.error ? (
                    <tr key={r.line} className="row-error">
                      <td className="muted">{r.line}</td>
                      <td colSpan={4} className="left down">
                        {r.error}
                      </td>
                    </tr>
                  ) : (
                    <tr key={r.line} className={r.duplicate ? "row-dup" : ""}>
                      <td className="muted">{r.line}</td>
                      <th scope="row">
                        <span className="asset">
                          <Coin symbol={r.base!.asset.symbol} />
                          <span>
                            <span className="sym">{KIND[r.kind!]}</span>
                            {r.duplicate && <span className="sub always">déjà là</span>}
                          </span>
                        </span>
                      </th>
                      <td className="mid muted">{r.time && date(r.time)}</td>
                      <td>
                        {amount(r.base!.qty, r.base!.asset.symbol)}
                      </td>
                      <td className="wide muted">{r.account || "–"}</td>
                    </tr>
                  ),
                )}
              </tbody>
            </table>
          </div>
          {preview.total > preview.rows.length && <p className="muted small-note">Aperçu des {preview.rows.length} premières lignes sur {preview.total}.</p>}

          {fresh > 0 && (
            <fieldset className="target">
              <legend>Dans quel compte ?</legend>
              {preview.fileAccounts && (
                <label className="radio">
                  <input type="radio" name="target" checked={fromFile} onChange={() => setFromFile(true)} />
                  <span>
                    Ceux du fichier <span className="muted">({preview.accounts.join(", ")})</span>
                  </span>
                </label>
              )}
              {preview.fileAccounts && (
                <label className="radio">
                  <input type="radio" name="target" checked={!fromFile} onChange={() => setFromFile(false)} />
                  <span>Un seul compte</span>
                </label>
              )}
              {(!fromFile || !preview.fileAccounts) && accounts.data && (
                <AccountPicker label="Compte" accounts={accounts.data} value={target} onChange={setTarget} />
              )}
            </fieldset>
          )}

          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          <div className="actions">
            <button type="button" className="btn primary" onClick={run} disabled={busy || !ready}>
              {busy ? "Import…" : fresh > 0 ? `Importer ${fresh} opération${fresh > 1 ? "s" : ""}` : "Rien de nouveau"}
            </button>
            <button type="button" className="btn quiet" onClick={reset}>
              Annuler
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
