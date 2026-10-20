// Package entry turns an operation as a person tells it ("bought 0.01 BTC
// for 550 €") into the activities Fundarr's engine replays, and back.
package entry

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/shopspring/decimal"
)

// Kinds of operation.
const (
	Buy        = "buy"        // crypto for euros
	Sell       = "sell"       // crypto for euros
	Swap       = "swap"       // crypto for crypto
	Deposit    = "deposit"    // euros (or coins) arriving from outside
	Withdrawal = "withdrawal" // euros (or coins) leaving
	Reward     = "reward"     // staking, airdrop
	Transfer   = "transfer"   // coins moved between two of your accounts
	Fee        = "fee"        // a charge on its own
	Correction = "correction" // Fundarr's balance check (read-only)
)

// Entry is an operation as the form (or a CSV row) gives it. Amounts are
// decimal strings, as typed ("1 234,56" works).
type Entry struct {
	Kind        string `json:"kind"`
	Date        string `json:"date"` // YYYY-MM-DD
	Account     string `json:"account,omitempty"`
	NewAccount  string `json:"newAccount,omitempty"` // made on the way
	To          string `json:"to,omitempty"`         // transfer: the receiving account
	NewTo       string `json:"newTo,omitempty"`
	Asset       string `json:"asset"`
	Qty         string `json:"qty"`
	Total       string `json:"total,omitempty"`       // buy, sell: euros paid or received
	Fee         string `json:"fee,omitempty"`         // euros; a transfer's network fee is in its coin
	Received    string `json:"received,omitempty"`    // swap: the coin received
	ReceivedQty string `json:"receivedQty,omitempty"` // swap
	Note        string `json:"note,omitempty"`
}

var eur = model.Asset{Kind: model.Fiat, Symbol: "EUR"}

var symbolRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9.]{0,19}$`)

// Asset reads a symbol: "btc" → crypto BTC, "EUR" → fiat.
func Asset(sym string) (model.Asset, error) {
	s := strings.ToUpper(strings.TrimSpace(sym))
	if !symbolRe.MatchString(s) {
		if s == "" {
			return model.Asset{}, errors.New("Actif manquant")
		}
		return model.Asset{}, errors.New("Actif invalide : " + sym)
	}
	return model.CryptoOrFiat(s), nil
}

// Decimal reads a number as people type it: "1 234,56", "0.01", "1,234.5".
func Decimal(s string) (decimal.Decimal, error) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == ' ' || r == ' ' || r == '€' {
			return -1
		}
		return r
	}, s)
	if strings.Contains(s, ",") {
		if strings.Contains(s, ".") {
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.ReplaceAll(s, ",", ".")
		}
	}
	return decimal.NewFromString(s)
}

func positive(s, what string) (decimal.Decimal, error) {
	if strings.TrimSpace(s) == "" {
		return decimal.Zero, errors.New(what + " manquant(e)")
	}
	d, err := Decimal(s)
	if err != nil {
		return d, errors.New(what + " illisible : " + s)
	}
	if !d.IsPositive() {
		return d, errors.New(what + " doit être positif(ve)")
	}
	return d, nil
}

// optional: "" or 0 is none.
func optional(s, what string) (decimal.Decimal, error) {
	if strings.TrimSpace(s) == "" {
		return decimal.Zero, nil
	}
	d, err := Decimal(s)
	if err != nil {
		return d, errors.New(what + " illisible : " + s)
	}
	if d.IsNegative() {
		return d, errors.New(what + " ne peut pas être négatif")
	}
	return d, nil
}

// Build returns the activities of the entry at time at, on account (and to,
// for a transfer: the withdrawal first, then the deposit), without ids.
func (e Entry) Build(at time.Time, account, to string) ([]model.Activity, error) {
	note := strings.TrimSpace(e.Note)
	if r := []rune(note); len(r) > 200 {
		note = string(r[:200])
	}
	act := model.Activity{SourceID: account, Time: at.UTC(), Note: note}
	asset := eur
	if strings.TrimSpace(e.Asset) != "" || (e.Kind != Deposit && e.Kind != Withdrawal) {
		a, err := Asset(e.Asset)
		if err != nil {
			return nil, err
		}
		asset = a
	}
	qty, err := positive(e.Qty, "Quantité")
	if err != nil {
		return nil, err
	}
	act.Base = model.Amount{Asset: asset, Qty: qty}
	crypto := func() error {
		if asset.Kind != model.Crypto {
			return errors.New("Choisis une crypto, pas " + asset.Symbol)
		}
		return nil
	}
	euroFee := func() error {
		f, err := optional(e.Fee, "Frais")
		if err != nil {
			return err
		}
		if f.IsPositive() {
			act.Fee = &model.Amount{Asset: eur, Qty: f}
		}
		return nil
	}
	switch e.Kind {
	case Buy, Sell:
		if err := crypto(); err != nil {
			return nil, err
		}
		total, err := positive(e.Total, "Prix")
		if err != nil {
			return nil, err
		}
		act.Type, act.Side = model.Trade, model.Buy
		if e.Kind == Sell {
			act.Side = model.Sell
		}
		act.Quote = &model.Amount{Asset: eur, Qty: total}
		if err := euroFee(); err != nil {
			return nil, err
		}
	case Swap:
		if err := crypto(); err != nil {
			return nil, err
		}
		got, err := Asset(e.Received)
		if err != nil {
			return nil, errors.New("Crypto reçue : " + err.Error())
		}
		if got.Kind != model.Crypto {
			return nil, errors.New("Pour des euros, c'est une vente")
		}
		if got.Key() == asset.Key() {
			return nil, errors.New("Choisis deux cryptos différentes")
		}
		gotQty, err := positive(e.ReceivedQty, "Quantité reçue")
		if err != nil {
			return nil, err
		}
		act.Type, act.Side = model.Trade, model.Sell
		act.Quote = &model.Amount{Asset: got, Qty: gotQty}
		if err := euroFee(); err != nil {
			return nil, err
		}
	case Deposit:
		act.Type = model.Deposit
	case Withdrawal:
		act.Type = model.Withdrawal
	case Reward:
		if err := crypto(); err != nil {
			return nil, err
		}
		act.Type = model.Reward
	case Fee:
		act.Type = model.Fee
	case Transfer:
		if err := crypto(); err != nil {
			return nil, err
		}
		if to == "" && account != "" {
			return nil, errors.New("Compte d'arrivée manquant")
		}
		if to == account && account != "" {
			return nil, errors.New("Choisis deux comptes différents")
		}
		f, err := optional(e.Fee, "Frais")
		if err != nil {
			return nil, err
		}
		out := act
		out.Type = model.Withdrawal
		if f.IsPositive() {
			out.Fee = &model.Amount{Asset: asset, Qty: f}
		}
		in := act
		in.Type, in.SourceID = model.Deposit, to
		return []model.Activity{out, in}, nil
	default:
		return nil, errors.New("Type d'opération inconnu")
	}
	return []model.Activity{act}, nil
}

// Kind says what an activity is, in the words of the form. paired: it is
// one side of a transfer between two accounts.
func Kind(a model.Activity, paired bool) string {
	switch a.Type {
	case model.Trade:
		if a.Quote != nil && a.Quote.Asset.Kind == model.Fiat && a.Base.Asset.Kind != model.Fiat {
			if a.Side == model.Sell {
				return Sell
			}
			return Buy
		}
		if a.Quote != nil && a.Base.Asset.Kind == model.Fiat {
			// Base euros, quote coins: the other way round.
			if a.Side == model.Sell {
				return Buy
			}
			return Sell
		}
		return Swap
	case model.Deposit, model.Withdrawal:
		if paired {
			return Transfer
		}
		if a.Type == model.Deposit {
			return Deposit
		}
		return Withdrawal
	case model.Reward, model.Interest, model.Dividend:
		return Reward
	case model.Fee, model.Tax:
		return Fee
	}
	return Correction
}
