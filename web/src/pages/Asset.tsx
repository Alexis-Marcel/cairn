// One asset, a coin or a security: its price with what I paid on average and my buys and sales,
// what I hold over time, my results on it, and its operations.

import { useEffect, useMemo, useState } from "react";
import { get, useLoad, type AssetDetail, type Op, type Ops, type PeriodKey } from "../api";
import { CLASS_ONE, label, longName, unit } from "../assets";
import { COINS } from "../coins";
import { Coin } from "../components/Coin";
import { LineChart } from "../components/LineChart";
import { Num } from "../components/Num";
import { OpList } from "../components/OpList";
import { Segmented } from "../components/Segmented";
import { Failed, Skel, SkelRows, Slow } from "../components/State";
import { day, eur, pct, price, qty, tone } from "../format";
import { DAY, utc } from "../lib/chart";
import { remember, remembered } from "../lib/remember";
import { Arrow, PERIODS, SINCE } from "./Home";

function startOf(key: PeriodKey, last: number): number {
  const d = new Date(last);
  const y = d.getUTCFullYear();
  const m = d.getUTCMonth();
  const dd = d.getUTCDate();
  switch (key) {
    case "7J":
      return last - 7 * DAY;
    case "1M":
      return Date.UTC(y, m - 1, dd);
    case "YTD":
      return Date.UTC(y - 1, 11, 31);
    case "1A":
      return Date.UTC(y - 1, m, dd);
  }
  return -Infinity;
}

export function Asset({ symbol, rev, go, onOpen }: { symbol: string; rev: number; go: (path: string) => void; onOpen: (op: Op) => void }) {
  const a = useLoad("asset" + symbol + rev, (s) => get<AssetDetail>("assets/" + encodeURIComponent(symbol), s));
  const [opsLimit, setOpsLimit] = useState(10);
  const ops = useLoad(`ops${symbol}${rev}${opsLimit}`, (s) => get<Ops>(`operations?asset=${encodeURIComponent(symbol)}&limit=${opsLimit}`, s));
  const [period, setPeriod] = useState<PeriodKey>(() => remembered("assetPeriod", "1A", ["7J", "1M", "YTD", "1A", "Tout"] as const));
  const [scrub, setScrub] = useState<number | null>(null);
  const [qScrub, setQScrub] = useState<number | null>(null);
  const known = a.data?.coin.asset;
  const name = known ? longName(known) : COINS.find((c) => c.symbol === symbol)?.name;
  useEffect(() => {
    if (known) document.title = `${label(known)} – Cairn`;
  }, [known]);

  const s = useMemo(() => {
    const pts = a.data?.points ?? [];
    if (!pts.length) return null;
    const all = pts.map((p) => utc(p.d));
    const from = startOf(period, all[all.length - 1]);
    // The opening: the last day on or before the period's eve.
    let i = all.findIndex((t) => t > from) - 1;
    if (i < 0) i = 0;
    // Long spans keep about 400 points, and the days of buys and sales.
    const marked = new Set((a.data?.markers ?? []).map((m) => m.d));
    const every = Math.ceil((pts.length - i) / 400);
    const keep = (n: number) => every <= 1 || (n - i) % every === 0 || n === pts.length - 1 || marked.has(pts[n].d);
    const idx = pts.map((_, n) => n).filter((n) => n >= i && keep(n));
    const part = idx.map((n) => pts[n]);
    const t = idx.map((n) => all[n]);
    // Buys and sales sit on the line, on their day.
    const at = new Map(t.map((x, n) => [x, n]));
    const marks = (a.data?.markers ?? [])
      .filter((m) => at.has(utc(m.d)))
      .map((m) => ({ t: utc(m.d), y: part[at.get(utc(m.d))!].p, buy: m.buy }));
    const byDay = new Map<string, { buy: number; sell: number }>();
    for (const m of a.data?.markers ?? []) {
      const e = byDay.get(m.d) ?? { buy: 0, sell: 0 };
      if (m.buy) e.buy += m.value;
      else e.sell += m.value;
      byDay.set(m.d, e);
    }
    return {
      part,
      t,
      p: part.map((x) => x.p),
      avg: part.some((x) => x.avg) ? part.map((x) => x.avg ?? NaN) : undefined,
      q: part.map((x) => x.q),
      marks,
      byDay,
    };
  }, [a.data, period]);

  if (a.error && !a.data) return <Failed error={a.error} retry={a.reload} />;
  const d = a.data;
  if (!d || !s) return <AssetSkeleton symbol={symbol} name={name} />;
  const c = d.coin;
  const sec = c.asset.kind === "security";
  const what = label(c.asset);
  const units = unit(c.asset);
  const first = s.part[0];
  const last = s.part[s.part.length - 1];
  const cur = scrub !== null ? s.part[scrub] : last;
  const change = cur.p - first.p;
  const changePct = first.p ? change / first.p : 0;

  return (
    <div className="asset-page">
      <a
        href="/actifs"
        className="back"
        onClick={(e) => {
          e.preventDefault();
          go("/actifs");
        }}
      >
        <svg viewBox="0 0 16 16" aria-hidden>
          <path d="m10 3-5 5 5 5" />
        </svg>
        Actifs
      </a>
      <section className="hero asset-hero" aria-labelledby="h-asset">
        <div className="asset-id">
          <Coin asset={c.asset} cls={c.class} size={44} />
          <div>
            <h1 id="h-asset">{name ?? what}</h1>
            <p className="muted">{sec ? `${CLASS_ONE[c.class]} · ${c.asset.isin ?? c.asset.symbol}` : name ? symbol : "Crypto"}</p>
          </div>
        </div>
        <p className="hero-when">{scrub !== null ? day(utc(cur.d)) : "Cours"}</p>
        <p className="hero-value num small">
          <Num value={cur.p} instant={scrub !== null} format={price} />
        </p>
        <p className={`hero-change num ${tone(change)}`}>
          <Arrow n={change} />
          <Num value={changePct} instant={scrub !== null} format={(v) => pct(v, { sign: true })} />
          <span className="since">{scrub !== null ? "depuis le " + day(utc(first.d)) : SINCE[period]}</span>
        </p>
      </section>

      <section className="graph" aria-label="Cours">
        <LineChart
          t={s.t}
          y={s.p}
          y2={c.stable ? undefined : s.avg}
          marks={s.marks.length <= 80 ? s.marks : undefined}
          tone={last.p >= first.p ? "up" : "down"}
          axis={(v) => (v >= 100 ? eur(v, { cents: false }) : price(v))}
          height={300}
          dataKey={period}
          label={`Cours de ${what} ${SINCE[period]}, avec le prix moyen payé et les achats et ventes. Flèches pour parcourir.`}
          scrub={scrub}
          onScrub={setScrub}
          tip={(i) => {
            const p = s.part[i];
            return (
              <dl>
                <div>
                  <dt>Cours</dt>
                  <dd>{price(p.p)}</dd>
                </div>
                {p.avg ? (
                  <div>
                    <dt>
                      <i className="dash" aria-hidden />
                      Prix moyen
                    </dt>
                    <dd>{price(p.avg)}</dd>
                  </div>
                ) : null}
                <div>
                  <dt>Détenu</dt>
                  <dd>{qty(p.q)}</dd>
                </div>
                {s.byDay.get(p.d)?.buy ? (
                  <div>
                    <dt>
                      <i className="dot buy" aria-hidden />
                      Acheté
                    </dt>
                    <dd>{eur(s.byDay.get(p.d)!.buy)}</dd>
                  </div>
                ) : null}
                {s.byDay.get(p.d)?.sell ? (
                  <div>
                    <dt>
                      <i className="dot sell" aria-hidden />
                      Vendu
                    </dt>
                    <dd>{eur(s.byDay.get(p.d)!.sell)}</dd>
                  </div>
                ) : null}
              </dl>
            );
          }}
        />
        <div className="graph-bar">
          <Segmented
            label="Période"
            className="chips"
            options={PERIODS}
            value={period}
            onChange={(v) => {
              setPeriod(v);
              setScrub(null);
              remember("assetPeriod", v);
            }}
          />
          <p className="legend muted">
            {!c.stable && (
              <span>
                <i className="dash" aria-hidden /> Prix moyen
              </span>
            )}
            <span>
              <i className="dot buy" aria-hidden /> Achat
            </span>
            <span>
              <i className="dot sell" aria-hidden /> Vente
            </span>
          </p>
        </div>
      </section>

      <section aria-labelledby="h-pos">
        <h2 id="h-pos">Ma position</h2>
        <dl className="figures num position">
          <div>
            <dt>Valeur</dt>
            <dd>{eur(c.value)}</dd>
            <dd className="figure-sub">
              {qty(c.qty)} {units}
            </dd>
          </div>
          <div>
            <dt>Prix moyen</dt>
            <dd>{c.avgCost ? price(c.avgCost) : "–"}</dd>
            <dd className="figure-sub">{c.held ? `coût ${eur(c.cost)}` : ""}</dd>
          </div>
          <div>
            <dt>Plus-value latente</dt>
            <dd className={tone(c.unrealized)}>{eur(c.unrealized, { sign: true })}</dd>
            <dd className="figure-sub">{c.cost > 0 && pct(c.unrealized / c.cost, { sign: true })}</dd>
          </div>
          <div>
            <dt>Plus-value réalisée</dt>
            <dd className={tone(c.realized)}>{eur(c.realized, { sign: true })}</dd>
            <dd className="figure-sub">ventes, frais déduits</dd>
          </div>
          {sec ? (
            <div>
              <dt>Dividendes</dt>
              <dd>{eur(c.dividends)}</dd>
              <dd className="figure-sub">
                {c.rewards > 0 ? `+ ${eur(c.rewards)} offerts` : c.fees > 0 ? `frais ${eur(c.fees)}` : "nets d'impôt"}
              </dd>
            </div>
          ) : (
            <div>
              <dt>Récompenses</dt>
              <dd>{eur(c.rewards)}</dd>
              <dd className="figure-sub">{c.fees > 0 ? `frais ${eur(c.fees)}` : ""}</dd>
            </div>
          )}
          <div className="total-gain">
            <dt>Gain total</dt>
            <dd className={tone(c.gain)}>{eur(c.gain, { sign: true })}</dd>
            <dd className="figure-sub">{c.pct !== undefined && pct(c.pct, { sign: true }) + " du prix payé"}</dd>
          </div>
        </dl>
      </section>

      {s.q.some((v) => v > 0) && (
        <section aria-labelledby="h-qty" className="qty-sec">
          <div className="sec-head">
            <h2 id="h-qty">Quantité détenue</h2>
            <span className="muted num">{qScrub !== null ? `${qty(s.q[qScrub])} ${units} le ${day(s.t[qScrub])}` : `${qty(c.qty)} ${units}`}</span>
          </div>
          <LineChart
            t={s.t}
            y={s.q}
            tone="ink"
            fromZero
            axis={(v) => qty(v)}
            height={150}
            dataKey={"q" + period}
            label={`Quantité de ${what} détenue ${SINCE[period]}`}
            scrub={qScrub}
            onScrub={setQScrub}
          />
        </section>
      )}

      <section aria-labelledby="h-ops">
        <div className="sec-head">
          <h2 id="h-ops">Opérations</h2>
          <a
            className="more-link"
            href={"/activite?actif=" + symbol}
            onClick={(e) => {
              e.preventDefault();
              go("/activite?actif=" + symbol);
            }}
          >
            Dans l'activité
          </a>
        </div>
        {ops.data ? (
          <>
            <OpList items={ops.data.items} onOpen={onOpen} sameAsset />
            {ops.data.total > ops.data.items.length && (
              <button type="button" className="btn more" onClick={() => setOpsLimit((l) => l + 40)} disabled={ops.loading}>
                Voir plus ({ops.data.total - ops.data.items.length})
              </button>
            )}
          </>
        ) : (
          <SkelRows rows={3} />
        )}
      </section>
    </div>
  );
}

function AssetSkeleton({ symbol, name }: { symbol: string; name?: string }) {
  return (
    <div className="asset-page" aria-busy="true">
      <section className="hero asset-hero">
        <div className="asset-id">
          {name || !/^[A-Z]{2}[A-Z0-9]{9}\d$/.test(symbol) ? <Coin symbol={symbol} size={44} /> : <Skel w={44} h={44} r={13} />}
          <div>{name || !/^[A-Z]{2}[A-Z0-9]{9}\d$/.test(symbol) ? <h1>{name ?? symbol}</h1> : <Skel w={220} h={28} r={8} />}</div>
        </div>
        <Skel w="min(300px, 70%)" h={44} r={10} className="skel-hero" />
        <Skel w={160} h={18} />
      </section>
      <div className="graph">
        <Skel w="100%" h={300} r={16} />
        <Slow />
      </div>
    </div>
  );
}
