# Cairn

Where my crypto stands: total value, what I put in, what it became, and the lines of the French 2086 tax form. A small personal app that reads everything from [Fundarr](https://github.com/Alexis-Marcel/fundarr)'s read API and stores nothing.

![Accueil](docs/screenshots/accueil-clair.png)

- **Accueil**: value, invested, gain; value against invested over time (1M, 1A, Tout); allocation by asset or platform; each asset; this month's money in and rewards.
- **Impôts**: one year of disposals as 2086 lines, totals, estimated tax, a CSV to copy into the form. Rewards can be counted as already taxed income.

One Go binary serves the UI and forwards its reads to Fundarr with the token, which never reaches the browser. Answers are cached for a minute.

**There is no sign-in.** Put Cairn behind a reverse proxy that does it (Authentik, Authelia, oauth2-proxy…), never straight on the internet.

## Run with Docker

Create a read token in Fundarr (Settings → Export and API), then:

```sh
docker run -d -p 127.0.0.1:8080:8080 \
  -e FUNDARR_URL=http://fundarr:9494 \
  -e FUNDARR_TOKEN=fdr_… \
  ghcr.io/alexis-marcel/cairn
```

or see [`compose.yaml`](compose.yaml).

| Variable | Default | |
|---|---|---|
| `FUNDARR_URL` | `http://localhost:9494` | Fundarr's address, as Cairn reaches it |
| `FUNDARR_TOKEN` | | Fundarr read token (`fdr_…`) |
| `CAIRN_ADDR` | `:8080` | address to listen on |
| `CAIRN_CACHE` | `60s` | how long an answer is kept (`0s`: never) |

`/healthz` answers `ok`. Only `GET` reaches Fundarr; Fundarr's errors come back as `{"error": "…"}`.

## Develop

Go 1.26 and Node 24. `fakefundarr` answers like Fundarr with a made-up portfolio (2023 to today, BTC, ETH, SOL, ADA, USDT, three platforms, sales every year):

```sh
go run ./cmd/fakefundarr                       # :9494, token fdr_dev
FUNDARR_URL=http://localhost:9494 FUNDARR_TOKEN=fdr_dev go run ./cmd/cairn
cd web && npm install && npm run dev           # http://localhost:5173
```

`go test ./...` and `npm run build` (typecheck included) are what CI runs. `go build ./cmd/cairn` after `npm run build` gives the binary with the UI inside.

## Credits

Coin icons: [cryptocurrency-icons](https://github.com/spothq/cryptocurrency-icons), CC0. Typeface: [Geologica](https://fonts.google.com/specimen/Geologica), OFL.

## Licence

AGPL-3.0. Voir [LICENSE](LICENSE).
