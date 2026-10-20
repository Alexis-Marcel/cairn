# Cairn

Where my crypto stands: total value, what I put in, what it became, and the lines of the French 2086 tax form. A small, opinionated personal app, crypto only for now: you give it your operations, it does the rest.

![Accueil](docs/screenshots/accueil-clair.png)

- **Accueil**: value, invested, gain; value against invested over time (1M, 1A, Tout); allocation by asset or account; each asset; this month's money in and rewards.
- **Opérations**: every operation, by account or asset, searchable. Add, change or remove those entered by hand or imported.
- **Impôts**: one year of disposals as 2086 lines, totals, estimated tax, a CSV to copy into the form. Rewards can be counted as already taxed income.

One Go binary serves the UI, keeps everything in SQLite (`/data/cairn.db`) and computes the portfolio itself, with [Fundarr](https://github.com/Alexis-Marcel/fundarr)'s engine as a library (average cost per account, transfers carrying their cost, the 2086 method). Daily prices come from Binance and OKX public market data and the ECB's rates (through Frankfurter): no key, no account, each day asked once and kept.

**Cairn has no sign-in of its own.** Keep it on localhost, or put it behind a reverse proxy that signs you in (Authentik, Authelia, oauth2-proxy…). Never straight on the internet: whoever opens it sees and edits the portfolio.

## Run with Docker

```sh
docker run -d --name cairn -p 127.0.0.1:8080:8080 -v cairn:/data ghcr.io/alexis-marcel/cairn
```

or see [`compose.yaml`](compose.yaml), then open http://localhost:8080. With a bind mount (`./data:/data`), the folder must be writable by the image's user: `mkdir -p data && sudo chown 65532:65532 data`.

## Three ways to feed it

Cairn opens on an empty Accueil that offers all three; they mix freely.

1. **By hand**: Ajouter. Achat, vente, échange, dépôt, retrait, récompense, transfert entre comptes, frais. A date, an account (or a new one), a coin, a quantity; the price of a purchase or a sale is filled in from the day's close.
2. **A CSV file**: Opérations → Importer un CSV. Each row is checked and shown before anything is added; importing the same file again adds nothing.
3. **Fundarr** (optional): Réglages → Synchroniser avec Fundarr. Cairn keeps a copy of Fundarr's history through its [changes feed](https://github.com/Alexis-Marcel/fundarr/blob/main/docs/api.md#get-apiv1changes), every 15 minutes, one account per Fundarr source. Those operations are read-only in Cairn: change them in Fundarr.

### Fundarr

In Fundarr, **Apps → Add app → Cairn** gives a token. In Cairn, Réglages: paste the token; the address fills in by itself when Fundarr runs next to Cairn (`http://fundarr:9494` on a shared Docker network). Use that internal address rather than a public one, which a proxy may block. Cairn tries them, then copies the history. **Délier** asks whether to keep the copied operations (they become editable) or remove them.

To be told of changes instead of waiting for the next 15 minutes: Réglages → *Être prévenu par Fundarr* shows Cairn's notify address (`https://cairn.example/api/fundarr/notify`). Give it to Fundarr (**Apps → Cairn → Notify**), then paste the secret Fundarr shows once into Cairn. Fundarr signs each call with it (`Fundarr-Signature`, HMAC-SHA256 of `<t>.<body>`, checked to be recent); without a secret the route answers 404.

Fundarr must be able to reach that address. Behind a signing-in proxy, **`POST /api/fundarr/notify` is the only route to let through without sign-in**: it changes nothing but asks Cairn to sync, and only with a valid signature. Everything else stays behind the sign-in.

### CSV format

Fundarr's own export (`GET /api/v1/activities.csv?format=fundarr`) is read as is: each row keeps its account (Fundarr's source) and transfers stay paired.

Or Cairn's template, served at `/api/import/modele.csv` (Importer → Télécharger le modèle): one operation per row, commas or semicolons, decimal points or commas, columns in any order.

| Column | |
|---|---|
| `date` | `2024-03-01`, `01/03/2024` or an RFC 3339 time |
| `type` | `achat`, `vente`, `echange`, `depot`, `retrait`, `recompense`, `frais` (or `buy`, `sell`, `swap`, `deposit`, `withdrawal`, `reward`, `fee`) |
| `actif`, `quantite` | `BTC`, `0,01` (`EUR` for cash deposits and withdrawals) |
| `prix` | `achat`, `vente`: the total in euros |
| `frais` | in euros, optional |
| `recu`, `quantite_recue` | `echange`: the coin received and how much |
| `compte`, `note` | optional; without `compte`, pick one account for the whole file |

```csv
date,type,actif,quantite,prix,frais,recu,quantite_recue,compte,note
2024-03-01,achat,BTC,0.01,550,1.5,,,Binance,
2024-05-10,echange,BTC,0.005,,,ETH,0.08,Binance,
2024-06-15,recompense,ETH,0.002,,,,,Binance,staking
```

## Settings

| Variable | Default | |
|---|---|---|
| `CAIRN_DATA` | `/data` | where `cairn.db` is kept (readable by Cairn alone: it holds the Fundarr token) |
| `CAIRN_ADDR` | `:8080` | address to listen on |
| `CAIRN_CACHE` | `60s` | how long a computed answer is kept while nothing changes (prices move) |
| `FUNDARR_URL` | | Fundarr's address; fixes the link, no page to change it |
| `FUNDARR_TOKEN` | | Fundarr read token (`fdr_…`), with `FUNDARR_URL` |
| `FUNDARR_NOTIFY_SECRET` | | the notify secret, instead of pasting it in Réglages |

`/healthz` answers `ok`. Writes under `/api` must be JSON sent by Cairn's own page (cross-origin protection); errors come back as `{"error": "…"}`. An earlier Cairn's link (`/data/cairn.json`) is moved into the database on first start.

## Develop

Go 1.26 and Node 24. Fundarr's demo gives a made-up portfolio (2023 to today, three sources, rewards, transfers, sales) to sync from; run it from a Fundarr clone:

```sh
FUNDARR_DEMO=1 FUNDARR_ADDR=127.0.0.1:9599 go run ./cmd/fundarr   # in fundarr/, token fdr_demo
CAIRN_DATA=./data go run ./cmd/cairn                               # :8080; Réglages → http://127.0.0.1:9599, fdr_demo
cd web && npm install && npm run dev                               # http://localhost:5173
```

`gofmt`, `go vet ./...`, `go test -race ./...` and `npm run build` (typecheck included) are what CI runs. `go build ./cmd/cairn` after `npm run build` gives the binary with the UI inside.

| | |
|---|---|
| ![Opérations](docs/screenshots/operations-clair.png) | ![Ajouter](docs/screenshots/ajouter-clair.png) |
| ![Accueil, sombre](docs/screenshots/accueil-sombre.png) | ![Mobile](docs/screenshots/operations-mobile-sombre.png) |
| ![Premier lancement](docs/screenshots/bienvenue-clair.png) | ![Impôts](docs/screenshots/impots-clair.png) |

## Credits

Coin icons: [cryptocurrency-icons](https://github.com/spothq/cryptocurrency-icons), CC0. Typeface: [Geologica](https://fonts.google.com/specimen/Geologica), OFL. Portfolio engine and prices: [Fundarr](https://github.com/Alexis-Marcel/fundarr)'s `pkg/`, AGPL-3.0.

## Licence

AGPL-3.0. Voir [LICENSE](LICENSE).
