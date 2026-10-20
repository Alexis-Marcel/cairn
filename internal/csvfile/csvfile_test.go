package csvfile

import (
	"strings"
	"testing"

	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

func TestTemplate(t *testing.T) {
	f, err := Read(strings.NewReader(Template))
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != Cairn || !f.Accounts || len(f.Rows) != 9 {
		t.Fatalf("%+v", f)
	}
	for _, r := range f.Rows {
		if r.Error != "" {
			t.Errorf("line %d: %s", r.Line, r.Error)
		}
	}
	buy := f.Rows[0].Activity
	if buy.Type != model.Trade || buy.Side != model.Buy || buy.Base.Qty.String() != "0.01" || buy.Quote.Qty.String() != "550" ||
		buy.Fee.Qty.String() != "1.5" || buy.Time.Format("2006-01-02 15") != "2024-03-01 12" {
		t.Fatalf("buy: %+v", buy)
	}
	swap := f.Rows[2].Activity
	if swap.Side != model.Sell || swap.Quote.Asset.Symbol != "ETH" || swap.Quote.Qty.String() != "0.08" {
		t.Fatalf("swap: %+v", swap)
	}
	if f.Rows[1].Activity.Base.Asset.Kind != model.Fiat {
		t.Fatal("a deposit of euros is cash")
	}
	etf := f.Rows[4].Activity
	if etf.Base.Asset.Kind != model.Security || etf.Base.Asset.ISIN != "IE00B4L5Y983" || etf.Base.Asset.Name != "iShares Core MSCI World" ||
		etf.Quote.Qty.String() != "300" {
		t.Fatalf("ETF: %+v", etf.Base)
	}
	div := f.Rows[8].Activity
	if div.Type != model.Dividend || div.Base.Asset.Symbol != "EUR" || div.Base.Qty.String() != "4.2" || div.Quote.Asset.ISIN != "IE00B4L5Y983" {
		t.Fatalf("dividend: %+v", div)
	}
}

// A security by its ISIN, in isin or in actif; a wrong check digit is
// refused, and a stock cannot be swapped like a coin.
func TestISIN(t *testing.T) {
	csv := "date,type,actif,isin,nom,quantite,prix,compte\n" +
		"2025-01-03,achat,,FR0000120271,TotalEnergies,10,600,TR\n" +
		"2025-01-04,achat,US0378331005,,Apple,2,360,TR\n" +
		"2025-01-05,achat,,FR0000120272,,1,60,TR\n" +
		"2025-03-01,dividende,,FR0000120271,,,4.8,TR\n"
	f, err := Read(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if f.Rows[0].Error != "" || f.Rows[0].Activity.Base.Asset.ISIN != "FR0000120271" || f.Rows[0].Activity.Base.Asset.Name != "TotalEnergies" {
		t.Fatalf("by isin: %+v", f.Rows[0])
	}
	if f.Rows[1].Error != "" || f.Rows[1].Activity.Base.Asset.Kind != model.Security {
		t.Fatalf("an ISIN in actif: %+v", f.Rows[1])
	}
	if !strings.Contains(f.Rows[2].Error, "ISIN invalide") {
		t.Fatalf("bad check digit: %+v", f.Rows[2])
	}
	if d := f.Rows[3].Activity; f.Rows[3].Error != "" || d.Type != model.Dividend || d.Base.Qty.String() != "4.8" {
		t.Fatalf("dividend: %+v", f.Rows[3])
	}
}

func TestFrenchSpreadsheet(t *testing.T) {
	csv := "\xef\xbb\xbfDate;Type;Actif;Quantité;Prix;Frais;Compte\r\n" +
		"01/03/2024;Achat;btc;0,01;1 550,50;;Ledger\r\n" +
		"01/03/2024;Achat;btc;0,01;1 550,50;;Ledger\r\n" +
		"02/03/2024;vol;BTC;1;;;Ledger\r\n" +
		"03/03/2024;vente;BTC;1;;;\r\n" +
		";;;;;;\r\n"
	f, err := Read(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Rows) != 4 {
		t.Fatalf("blank rows skipped: %d", len(f.Rows))
	}
	a, b := f.Rows[0].Activity, f.Rows[1].Activity
	if a.Quote.Qty.String() != "1550.5" || a.Base.Asset.Symbol != "BTC" {
		t.Fatalf("%+v", a)
	}
	if a.ID == b.ID || a.ID == "" {
		t.Fatal("two identical rows are two operations")
	}
	if !a.Time.Before(b.Time) {
		t.Fatal("rows of a day keep their order")
	}
	if f.Rows[2].Error == "" || f.Rows[3].Error == "" {
		t.Fatalf("errors: %+v", f.Rows[2:])
	}
	// The same file gives the same ids: importing it again adds nothing.
	g, _ := Read(strings.NewReader(csv))
	if g.Rows[0].Activity.ID != a.ID || g.Rows[1].Activity.ID != b.ID {
		t.Fatal("ids depend on the content only")
	}
}

func TestFundarrExport(t *testing.T) {
	csv := `id,source,account,time,type,side,qty,asset,isin,quote_qty,quote_asset,fee_qty,fee_asset,cost,currency,transfer_with,network,tx_id,address,note
b1,Binance,,2024-02-05T09:12:00Z,trade,buy,0.01,BTC,,400,EUR,0.2,EUR,400.00,EUR,,,,,
w1,Binance,,2024-03-01T10:00:00Z,withdrawal,,0.01,BTC,,,,,,,,d1,BTC,abc,,
d1,Ledger,,2024-03-01T10:30:00Z,deposit,,0.01,BTC,,,,,,,,w1,BTC,abc,,
x1,Binance,,nope,trade,buy,1,BTC,,,,,,,,,,,,
`
	f, err := Read(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if f.Format != Fundarr || len(f.Rows) != 4 {
		t.Fatalf("%+v", f)
	}
	d := f.Rows[2]
	if d.Activity.ID != "d1" || d.Activity.TransferWith != "w1" || d.Account != "Ledger" || d.Activity.External != "d1" {
		t.Fatalf("deposit: %+v", d)
	}
	if f.Rows[3].Error == "" {
		t.Fatal("bad time")
	}
}

func TestUnknownColumns(t *testing.T) {
	if _, err := Read(strings.NewReader("foo,bar\n1,2\n")); err == nil {
		t.Fatal("refused")
	}
	if _, err := Read(strings.NewReader("")); err == nil {
		t.Fatal("empty")
	}
}
