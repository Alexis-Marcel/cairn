// Package csvfile reads activities from a CSV file: Fundarr's export
// (GET /api/v1/activities.csv?format=fundarr) or Cairn's own simple format,
// one operation per row:
//
//	date,type,actif,quantite,prix,frais,recu,quantite_recue,compte,note,isin,nom
//	2024-03-01,achat,BTC,0.01,550,1.5,,,Binance,,,
//	2024-05-10,echange,BTC,0.005,,,ETH,0.08,Binance,,,
//	2024-07-01,achat,,3,300,1,,,Trade Republic,,IE00B4L5Y983,iShares Core MSCI World
//
// A stock or an ETF is named by its ISIN (in isin, or in actif), with its
// name in nom.
// Columns may come in any order, separated by commas or semicolons, with
// decimal points or commas. Every row has an id that depends only on its
// content, so importing the same file twice adds nothing the second time.
package csvfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/entry"
	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/shopspring/decimal"
)

// Formats.
const (
	Fundarr = "fundarr"
	Cairn   = "cairn"
)

// MaxRows bounds one file.
const MaxRows = 50000

// Row is one row read: an activity, or why it could not be.
type Row struct {
	Line     int            `json:"line"`
	Activity store.Activity `json:"-"`
	Account  string         `json:"account,omitempty"` // as the file names it
	Error    string         `json:"error,omitempty"`
}

// File is what was read.
type File struct {
	Format string
	Rows   []Row
	// Accounts: every row that could be read names its account.
	Accounts bool
}

// Template is an example of Cairn's format, to fill in.
const Template = "date,type,actif,quantite,prix,frais,recu,quantite_recue,compte,note,isin,nom\r\n" +
	"2024-03-01,achat,BTC,0.01,550,1.5,,,Binance,,,\r\n" +
	"2024-04-02,depot,EUR,500,,,,,Binance,virement,,\r\n" +
	"2024-05-10,echange,BTC,0.005,,,ETH,0.08,Binance,,,\r\n" +
	"2024-06-15,recompense,ETH,0.002,,,,,Binance,staking,,\r\n" +
	"2024-07-01,achat,,3,300,1,,,Trade Republic,,IE00B4L5Y983,iShares Core MSCI World\r\n" +
	"2024-09-20,vente,ETH,0.05,120,0.5,,,Binance,,,\r\n" +
	"2024-10-01,frais,ETH,0.001,,,,,Binance,frais de garde,,\r\n" +
	"2024-11-05,retrait,BTC,0.002,,,,,Binance,envoyé à un ami,,\r\n" +
	"2024-12-10,dividende,,,4.2,,,,Trade Republic,,IE00B4L5Y983,\r\n"

// fold lowers a header and drops its accents and spaces: "Quantité reçue" →
// "quantite_recue".
func fold(s string) string {
	return strings.NewReplacer(" ", "_", "-", "_", "'", "_", "é", "e", "è", "e", "ê", "e", "ë", "e", "à", "a", "â", "a",
		"ç", "c", "ù", "u", "û", "u", "ô", "o", "î", "i", "ï", "i").Replace(strings.ToLower(strings.TrimSpace(s)))
}

// Read reads a whole file.
func Read(r io.Reader) (*File, error) {
	raw, err := io.ReadAll(io.LimitReader(r, 32<<20))
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("Fichier vide")
	}
	first, _, _ := bytes.Cut(raw, []byte("\n"))
	cr := csv.NewReader(bytes.NewReader(raw))
	if bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return nil, errors.New("Fichier CSV illisible")
	}
	col := map[string]int{}
	for i, h := range header {
		if _, ok := col[fold(h)]; !ok {
			col[fold(h)] = i
		}
	}
	f := &File{}
	var parse func(get func(string) string) (store.Activity, string, error)
	switch {
	case has(col, "id", "source", "time", "type", "qty", "asset"):
		f.Format, parse = Fundarr, fundarrRow
	case has(col, "date", "type") && (has(col, "actif") || has(col, "asset") || has(col, "isin")):
		f.Format, parse = Cairn, cairnRow
	default:
		return nil, errors.New("Colonnes inconnues : utilise le modèle de Cairn ou l'export de Fundarr")
	}
	seen := map[string]int{}
	f.Accounts = true
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				f.Rows = append(f.Rows, Row{Line: pe.Line, Error: "Ligne illisible"})
				continue
			}
			return nil, err
		}
		if blank(rec) {
			continue
		}
		if len(f.Rows) >= MaxRows {
			return nil, fmt.Errorf("Plus de %d lignes : coupe le fichier", MaxRows)
		}
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		a, account, err := parse(get)
		row := Row{Line: line, Account: account}
		if err != nil {
			row.Error = err.Error()
		} else {
			if f.Format == Cairn {
				// The same row twice is two operations: the n-th copy has an id of its own.
				key := strings.Join(rec, "\x1f")
				seen[key]++
				h := sha256.Sum256([]byte(fmt.Sprintf("%s\x1e%d", key, seen[key])))
				a.External = hex.EncodeToString(h[:12])
				a.ID = "i-" + a.External
				// Rows of one day keep their order.
				a.Time = a.Time.Add(time.Duration(line) * time.Millisecond)
			}
			a.Origin = store.Import
			row.Activity = a
			if account == "" {
				f.Accounts = false
			}
		}
		f.Rows = append(f.Rows, row)
	}
	if len(f.Rows) == 0 {
		return nil, errors.New("Aucune ligne")
	}
	return f, nil
}

func has(col map[string]int, names ...string) bool {
	for _, n := range names {
		if _, ok := col[n]; !ok {
			return false
		}
	}
	return true
}

func blank(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// fundarrRow: Fundarr's export, every field.
func fundarrRow(get func(string) string) (store.Activity, string, error) {
	var a store.Activity
	id := get("id")
	if id == "" {
		return a, "", errors.New("id manquant")
	}
	t, err := time.Parse(time.RFC3339, get("time"))
	if err != nil {
		return a, "", errors.New("Date illisible")
	}
	typ := model.Type(get("type"))
	switch typ {
	case model.Trade, model.Deposit, model.Withdrawal, model.Reward, model.Fee, model.Dividend, model.Interest, model.Tax, model.Adjustment:
	default:
		return a, "", fmt.Errorf("Type inconnu : %s", get("type"))
	}
	amount := func(qty, sym, isin string) (*model.Amount, error) {
		if qty == "" && sym == "" {
			return nil, nil
		}
		q, err := decimal.NewFromString(qty)
		if err != nil || q.IsNegative() {
			return nil, fmt.Errorf("Quantité illisible : %s", qty)
		}
		as := model.CryptoOrFiat(sym)
		if isin == "" && entry.ValidISIN(as.Symbol) {
			isin = as.Symbol // a dividend's security, a fee in shares
		}
		if isin != "" {
			as = model.Asset{Kind: model.Security, Symbol: as.Symbol, ISIN: isin}
		}
		if as.Symbol == "" && as.ISIN == "" {
			return nil, errors.New("Actif manquant")
		}
		return &model.Amount{Asset: as, Qty: q}, nil
	}
	base, err := amount(get("qty"), get("asset"), get("isin"))
	if err != nil {
		return a, "", err
	}
	if base == nil {
		return a, "", errors.New("Quantité manquante")
	}
	quote, err := amount(get("quote_qty"), get("quote_asset"), "")
	if err != nil {
		return a, "", err
	}
	fee, err := amount(get("fee_qty"), get("fee_asset"), "")
	if err != nil {
		return a, "", err
	}
	if typ == model.Trade && quote == nil {
		return a, "", errors.New("Échange sans contrepartie")
	}
	a.Activity = model.Activity{ID: id, NativeID: id, Account: get("account"), Time: t.UTC(), Type: typ,
		Side: model.Side(get("side")), Base: *base, Quote: quote, Fee: fee,
		Network: get("network"), TxID: get("tx_id"), Address: get("address"), Note: get("note")}
	a.External, a.TransferWith = id, get("transfer_with")
	return a, get("source"), nil
}

// Kinds of Cairn's format, French or English.
var kinds = map[string]string{
	"achat": "buy", "buy": "buy",
	"vente": "sell", "sell": "sell",
	"echange": "swap", "swap": "swap", "conversion": "swap",
	"depot": "deposit", "deposit": "deposit",
	"retrait": "withdrawal", "withdrawal": "withdrawal",
	"recompense": "reward", "reward": "reward", "staking": "reward", "airdrop": "reward",
	"frais": "fee", "fee": "fee",
	"dividende": "dividend", "dividend": "dividend", "distribution": "dividend",
	"interets": "interest", "interet": "interest", "interest": "interest",
}

// Date reads a day or a moment: 2024-03-01, 01/03/2024, RFC 3339. A day is
// taken at noon UTC, so it stays the same day everywhere.
func Date(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "02/01/2006 15:04", "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	for _, layout := range []string{time.DateOnly, "02/01/2006", "2/1/2006", "02.01.2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Add(12 * time.Hour).UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("Date illisible : %s", s)
}

// cairnRow: Cairn's simple format.
func cairnRow(get func(string) string) (store.Activity, string, error) {
	pick := func(names ...string) string {
		for _, n := range names {
			if v := get(n); v != "" {
				return v
			}
		}
		return ""
	}
	var a store.Activity
	account := pick("compte", "account", "plateforme")
	t, err := Date(get("date"))
	if err != nil {
		return a, account, err
	}
	kind := kinds[fold(get("type"))]
	if kind == "" {
		return a, account, fmt.Errorf("Type inconnu : %s", get("type"))
	}
	e := entry.Entry{Kind: kind, Asset: pick("isin", "actif", "asset"), Name: pick("nom", "name", "libelle"),
		Qty: pick("quantite", "qty", "quantity"), Total: pick("prix", "prix_total", "total", "montant", "price"),
		Fee: pick("frais", "fee", "fees"), Received: pick("recu", "actif_recu", "received"),
		ReceivedQty: pick("quantite_recue", "received_qty"), Note: pick("note")}
	if (kind == entry.Dividend || kind == entry.Interest) && e.Qty == "" {
		// Euros received: in the price column, as a sale's.
		e.Qty = e.Total
	}
	acts, err := e.Build(t, "", "")
	if err != nil {
		return a, account, err
	}
	a.Activity = acts[0]
	return a, account, nil
}
