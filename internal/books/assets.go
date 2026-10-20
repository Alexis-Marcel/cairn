package books

// What an asset is, for the pages: its class (crypto, stablecoin, stock,
// ETF, cash) and the name it is shown with.

import (
	"regexp"
	"strings"

	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

// Classes of asset.
const (
	ClassCrypto = "crypto"
	ClassStable = "stable" // stablecoins
	ClassStock  = "stock"  // shares of a company
	ClassETF    = "etf"    // ETFs and funds
	ClassCash   = "cash"   // fiat
)

var isinRe = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{9}[0-9]$`)

// Words that name a fund, not a company.
var fundRe = regexp.MustCompile(`(?i)\b(etf|etc|etn|ucits|ishares|amundi|lyxor|xtrackers|vanguard|spdr|invesco|wisdomtree|vaneck|ossiam|bnp paribas easy|hsbc msci|ubs etf|franklin ftse|msci|s&p ?500|stoxx|ftse|nasdaq[- ]?100|index|fund|fonds|sicav)\b`)

// Class of an asset. Stocks and ETFs are told apart by their name (Fundarr
// does not say); without one, an Irish or Luxembourg ISIN is a fund.
func Class(a model.Asset) string {
	switch {
	case a.Kind == model.Fiat:
		return ClassCash
	case a.Kind == model.Crypto && model.PeggedTo(a) != "":
		return ClassStable
	case a.Kind == model.Crypto:
		return ClassCrypto
	}
	if a.Name != "" {
		if fundRe.MatchString(a.Name) {
			return ClassETF
		}
		return ClassStock
	}
	isin := a.ISIN
	if isin == "" {
		isin = a.Symbol
	}
	if strings.HasPrefix(isin, "IE") || strings.HasPrefix(isin, "LU") {
		return ClassETF
	}
	return ClassStock
}

// security fills in what a security's activities left out: its ISIN when
// its symbol is one, its name when another activity gave it.
func security(a *model.Asset, names map[string]string) {
	if a.Kind == model.Security && a.ISIN == "" && isinRe.MatchString(a.Symbol) {
		a.ISIN = a.Symbol
	}
	if a.Name == "" && a.Kind != model.Fiat {
		a.Name = names[a.Key()]
	}
}

// named gives every asset of the activities the name one of them carries
// (Fundarr passes on the platform's: "iShares Core MSCI World").
func named(acts []model.Activity) {
	names := map[string]string{}
	each := func(f func(*model.Asset)) {
		for i := range acts {
			a := &acts[i]
			f(&a.Base.Asset)
			if a.Quote != nil {
				q := *a.Quote
				f(&q.Asset)
				a.Quote = &q
			}
			if a.Fee != nil {
				x := *a.Fee
				f(&x.Asset)
				a.Fee = &x
			}
		}
	}
	each(func(x *model.Asset) {
		security(x, nil)
		if n := strings.TrimSpace(x.Name); n != "" && x.Kind != model.Fiat {
			names[x.Key()] = n
		}
	})
	each(func(x *model.Asset) { security(x, names) })
}

// AssetID is how pages and links name an asset: its ISIN for a security,
// its symbol otherwise.
func AssetID(a model.Asset) string {
	if a.Kind == model.Security && a.ISIN != "" {
		return a.ISIN
	}
	return a.Symbol
}

// Label is an asset as text: its name, or its symbol.
func Label(a model.Asset) string {
	if a.Name != "" && a.Kind == model.Security {
		return a.Name
	}
	return a.Symbol
}
