// Impôts: one year of crypto sales for euros, as the lines of form 2086,
// and a CSV of them to copy into the form. Stocks and ETFs are left out.

import { useMemo, useState } from "react";
import { get, useLoad, type History, type TaxFR } from "../api";
import { Coin } from "../components/Coin";
import { TaxCards } from "../components/PlanCards";
import { Segmented } from "../components/Segmented";
import { Failed, Loading } from "../components/State";
import { date, eur, num, pct, tone } from "../format";

/**
 * The CSV of the lines, for a French spreadsheet (";", decimal commas), one
 * column per box of the 2086, in whole euros as the form takes them.
 */
function csv(t: TaxFR): string {
  const n = (s: string | number) => String(Math.round(num(s)));
  const day = (iso: string) => new Date(iso).toLocaleDateString("fr-FR");
  const rows = [
    [
      "Date de cession (211)",
      "Actif",
      "Plateforme",
      "Quantité",
      "Valeur du portefeuille (212)",
      "Prix de cession (213)",
      "Frais (214)",
      "Prix net des frais (215)",
      "Soulte (216)",
      "Prix net des soultes (217)",
      "Prix net des frais et soultes (218)",
      "Prix total d'acquisition net (223)",
      "Plus-value (224)",
      "Fraction de capital (221 des lignes suivantes)",
    ],
    ...t.disposals.map((d) => [
      day(d.time),
      d.asset.symbol,
      d.source.name,
      d.qty.replace(".", ","),
      n(d.portfolio),
      n(d.price),
      n(d.fees),
      n(num(d.price) - num(d.fees)),
      "0",
      n(d.price),
      n(num(d.price) - num(d.fees)),
      n(d.acquisition),
      n(d.gain),
      n(d.capitalShare),
    ]),
  ];
  const cell = (s: string) => (/[;"\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s);
  return "﻿" + rows.map((r) => r.map(cell).join(";")).join("\r\n") + "\r\n";
}

function download(t: TaxFR, income: boolean) {
  const url = URL.createObjectURL(new Blob([csv(t)], { type: "text/csv;charset=utf-8" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = `cairn-2086-${t.year}${income ? "-revenu" : ""}.csv`;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function Taxes({ rev }: { rev: number }) {
  const span = useLoad("span" + rev, (s) => get<History>("portfolio/history?step=month", s));
  const years = useMemo(() => {
    const now = new Date().getFullYear();
    const first = span.data?.points[0]?.date;
    const from = first ? Number(first.slice(0, 4)) : now;
    return Array.from({ length: now - from + 1 }, (_, n) => from + n);
  }, [span.data]);

  // The year to declare is the last one.
  const [picked, setPicked] = useState<number | null>(null);
  const year = picked ?? (years.includes(new Date().getFullYear() - 1) ? new Date().getFullYear() - 1 : years[years.length - 1]);
  const [income, setIncome] = useState(false);

  const q = `tax/fr?year=${year}${income ? "&rewards=income" : ""}`;
  const tax = useLoad(span.data ? q + "#" + rev : null, (s) => get<TaxFR>(q, s));

  if (span.error) return <Failed error={span.error} retry={span.reload} />;
  if (!span.data) return <Loading />;

  const t = tax.data;
  const gain = t ? num(t.total.gain) : 0;

  return (
    <div className="taxes">
      <div className="tax-head">
        <h1>Impôts</h1>
        {years.length > 1 && (
          <Segmented label="Année" className="chips" options={years.map((y) => ({ value: y, label: String(y) }))} value={year} onChange={setPicked} />
        )}
      </div>

      <p className="muted tax-scope">Formulaire 2086 : cryptos seulement, actions et ETF n'y entrent pas.</p>

      <label className="switch">
        <input type="checkbox" checked={income} onChange={(e) => setIncome(e.target.checked)} />
        <span className="track" aria-hidden />
        Récompenses imposées comme revenu
      </label>

      {tax.error && !t ? (
        <Failed error={tax.error} retry={tax.reload} />
      ) : !t ? (
        <Loading />
      ) : (
        <div className={tax.loading ? "stale" : ""} aria-busy={tax.loading}>
          <dl className="tax-totals num">
            <div>
              <dt>Cessions</dt>
              <dd>{eur(t.total.disposals, { cents: true })}</dd>
            </div>
            <div>
              <dt>{gain < 0 ? "Moins-value" : "Plus-value"}</dt>
              <dd className={tone(gain)}>{eur(gain, { cents: true, sign: true })}</dd>
            </div>
            <div className="due">
              <dt>Impôt estimé</dt>
              {t.total.exempt ? (
                <dd>
                  Exonéré <span className="rate">cessions ≤ 305 €</span>
                </dd>
              ) : (
                <dd>
                  {eur(t.total.tax, { cents: true })} <span className="rate">{pct(num(t.total.rate), { rate: true })}</span>
                </dd>
              )}
            </div>
          </dl>

          {t.disposals.length === 0 ? (
            <p className="empty">Aucune cession imposable en {t.year}.</p>
          ) : (
            <>
              <div className="scroll">
                <table className="table num tax-table">
                  <thead>
                    <tr>
                      <th scope="col">Date</th>
                      <th scope="col">Actif</th>
                      <th scope="col">Prix de cession</th>
                      <th scope="col">Valeur du portefeuille</th>
                      <th scope="col">Fraction de capital</th>
                      <th scope="col">Plus-value</th>
                    </tr>
                  </thead>
                  <tbody>
                    {t.disposals.map((d, n) => (
                      <tr key={d.time + n}>
                        <td className="date" data-label="Date">{date(d.time)}</td>
                        <th scope="row">
                          <span className="asset">
                            <Coin symbol={d.asset.symbol} />
                            <span>
                              <span className="sym">{d.asset.symbol}</span>
                              <span className="sub always">{d.source.name}</span>
                            </span>
                          </span>
                        </th>
                        <td data-label="Prix de cession">
                          {eur(d.price, { cents: true })}
                          {num(d.fees) > 0 && <span className="sub always">frais {eur(d.fees, { cents: true })}</span>}
                        </td>
                        <td className="muted" data-label="Valeur du portefeuille">{eur(d.portfolio, { cents: true })}</td>
                        <td className="muted" data-label="Fraction de capital">{eur(d.capitalShare, { cents: true })}</td>
                        <td className={tone(num(d.gain))} data-label="Plus-value">{eur(d.gain, { cents: true, sign: true })}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <button type="button" className="btn" onClick={() => download(t, income)}>
                <svg viewBox="0 0 16 16" aria-hidden>
                  <path d="M8 2v8m0 0 3.5-3.5M8 10 4.5 6.5M2.5 13.5h11" />
                </svg>
                Télécharger le CSV
              </button>
            </>
          )}

          {t.notes.some(warning) && (
            <ul className="notes warn">
              {t.notes.filter(warning).map((n) => (
                <li key={n}>{n}</li>
              ))}
            </ul>
          )}
          {t.notes.some((n) => !warning(n)) && (
            <details className="notes-more">
              <summary>Comment c'est calculé</summary>
              <ul className="notes">
                {t.notes
                  .filter((n) => !warning(n))
                  .map((n) => (
                    <li key={n}>{n}</li>
                  ))}
              </ul>
            </details>
          )}
        </div>
      )}
      <TaxCards rev={rev} />
    </div>
  );
}

/** A note about this portfolio (a coin without a price), not about the method. */
const warning = (n: string) => n.startsWith("Pas de cours");
