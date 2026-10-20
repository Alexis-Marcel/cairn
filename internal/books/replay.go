package books

// The history replayed asset by asset: how much of each coin is held, what
// it cost, what it gained, what it earned and what was paid in fees. The
// costs are pkg/model's (Books: average cost per account, transfers
// carrying their cost); the gains follow pkg/portfolio's rules, split by
// coin.

import (
	"time"

	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/Alexis-Marcel/fundarr/pkg/portfolio"
	"github.com/shopspring/decimal"
)

// tally is one coin, so far.
type tally struct {
	Asset model.Asset
	held  map[string]decimal.Decimal // by account
	// Cost of what is held, across accounts.
	Cost decimal.Decimal
	// Realized: gains on disposals for cash or stablecoins, minus fees.
	// Rewards: what came in for nothing, at its day's value. Fees: paid on
	// operations in this coin, in euros. Bought: cost of what was bought or
	// swapped in, or deposited from outside.
	Realized, Rewards, Fees, Bought decimal.Decimal
	First, Last                     time.Time // first and last operation
}

// Qty held, across accounts (an account showing more leaving than
// arriving holds nothing, as in pkg/portfolio).
func (t *tally) Qty() decimal.Decimal {
	q := decimal.Zero
	for _, v := range t.held {
		if v.IsPositive() {
			q = q.Add(v)
		}
	}
	return q
}

// Marker is a buy or a sale of a coin, for its chart.
type Marker struct {
	Time  time.Time
	Buy   bool
	Asset string // the coin's key
	Qty   decimal.Decimal
	Value decimal.Decimal // in euros
}

// replay walks the history in pkg/model's order.
type replay struct {
	price portfolio.Price
	book  *model.Book
	pairs map[string]string // deposit → withdrawal
	out   map[string]bool   // withdrawals paired with a deposit
	coins map[string]*tally
	// Fees: every fee paid, in euros. Rewards: every reward.
	Fees, Rewards decimal.Decimal
	Markers       []Marker
	// RewardDays: each reward's value, by day, for the cumulative chart.
	RewardDays map[string]decimal.Decimal
}

func newReplay(l *Ledger, price portfolio.Price) *replay {
	value := func(a model.Amount, at time.Time) (decimal.Decimal, bool) {
		if a.Qty.IsZero() {
			return decimal.Zero, true
		}
		u, ok := price(a.Asset, at)
		return u.Mul(a.Qty), ok
	}
	r := &replay{price: price, book: model.Books(l.Acts, l.Pairs, value), pairs: l.Pairs, out: map[string]bool{},
		coins: map[string]*tally{}, RewardDays: map[string]decimal.Decimal{}}
	for _, o := range l.Pairs {
		r.out[o] = true
	}
	return r
}

func crypto(a model.Asset) bool { return a.Kind == model.Crypto }

// worth of an amount on a day; unpriced counts as 0.
func (r *replay) worth(a model.Amount, at time.Time) decimal.Decimal {
	if a.Qty.IsZero() {
		return decimal.Zero
	}
	u, _ := r.price(a.Asset, at)
	return u.Mul(a.Qty)
}

// paid: what a coin was traded for, in euros: the cash or stablecoins
// exchanged when there were some, the coin's close otherwise.
func (r *replay) paid(other, coin model.Amount, at time.Time) decimal.Decimal {
	if model.CashLike(other.Asset) {
		return r.worth(other, at)
	}
	return r.worth(coin, at)
}

func (r *replay) coin(a model.Asset, at time.Time) *tally {
	k := a.Key()
	t := r.coins[k]
	if t == nil {
		t = &tally{Asset: a, held: map[string]decimal.Decimal{}, First: at}
		r.coins[k] = t
	}
	t.Last = at
	return t
}

// move adds a signed quantity of a crypto to an account.
func (r *replay) move(src string, a model.Amount, sign int64, at time.Time) *tally {
	if !crypto(a.Asset) {
		return nil
	}
	t := r.coin(a.Asset, at)
	t.held[src] = t.held[src].Add(a.Qty.Mul(decimal.NewFromInt(sign)))
	return t
}

// addCost moves a coin's cost; it never goes below nothing, and nothing
// held costs nothing.
func addCost(t *tally, d decimal.Decimal) {
	if t == nil || model.CashLike(t.Asset) {
		return
	}
	t.Cost = t.Cost.Add(d)
	if t.Cost.IsNegative() || !t.Qty().IsPositive() {
		t.Cost = decimal.Zero
	}
}

func (r *replay) external(a model.Activity) bool {
	switch a.Type {
	case model.Deposit:
		return r.pairs[a.ID] == ""
	case model.Withdrawal:
		return !r.out[a.ID]
	}
	return a.Type == model.Adjustment
}

// apply replays one activity.
func (r *replay) apply(a model.Activity) {
	day := a.Time.UTC().Format(time.DateOnly)
	var feeOn *tally // the coin a fee is counted against
	switch {
	case a.Type == model.Trade && a.Quote != nil:
		got, given := a.Base, *a.Quote
		if a.Side == model.Sell {
			got, given = *a.Quote, a.Base
		}
		if !crypto(got.Asset) && !crypto(given.Asset) {
			break
		}
		net := got
		if a.Fee != nil && a.Fee.Asset.Key() == got.Asset.Key() {
			net.Qty = net.Qty.Sub(a.Fee.Qty)
		}
		tg := r.move(a.SourceID, got, 1, a.Time)
		tv := r.move(a.SourceID, given, -1, a.Time)
		if tv != nil {
			addCost(tv, r.book.Taken[a.ID].Neg())
			if model.CashLike(got.Asset) {
				tv.Realized = tv.Realized.Add(r.worth(net, a.Time)).Sub(r.book.Taken[a.ID])
			}
			if !model.CashLike(given.Asset) {
				r.Markers = append(r.Markers, Marker{a.Time, false, given.Asset.Key(), given.Qty, r.paid(got, given, a.Time)})
			}
			feeOn = tv
		}
		if tg != nil {
			if c, ok := r.book.Costs[a.ID]; ok {
				addCost(tg, c)
				tg.Bought = tg.Bought.Add(c)
			}
			if !model.CashLike(got.Asset) {
				r.Markers = append(r.Markers, Marker{a.Time, true, got.Asset.Key(), got.Qty, r.paid(given, got, a.Time)})
				feeOn = tg
			}
		}
		if feeOn != nil && a.Fee != nil {
			// Taken from the pool apart (pkg/portfolio counts it against the gain).
			feeOn.Realized = feeOn.Realized.Sub(r.book.Fees[a.ID])
		}
	case a.Inflow():
		t := r.move(a.SourceID, a.Base, 1, a.Time)
		if t == nil {
			break
		}
		c, ok := r.book.Costs[a.ID]
		if ok {
			addCost(t, c)
		}
		switch {
		case a.Type != model.Deposit && a.Type != model.Adjustment:
			v := r.worth(a.Base, a.Time)
			t.Rewards, r.Rewards = t.Rewards.Add(v), r.Rewards.Add(v)
			r.RewardDays[day] = r.RewardDays[day].Add(v)
		case r.external(a) && ok:
			t.Bought = t.Bought.Add(c)
		}
		feeOn = t
	case a.Outflow():
		if a.Type == model.Fee {
			r.Fees = r.Fees.Add(r.worth(a.Base, a.Time))
		}
		t := r.move(a.SourceID, a.Base, -1, a.Time)
		if t == nil {
			break
		}
		taken := r.book.Taken[a.ID]
		addCost(t, taken.Neg())
		switch {
		case a.Type == model.Fee || a.Type == model.Tax:
			t.Realized = t.Realized.Sub(taken)
			if a.Type == model.Fee {
				t.Fees = t.Fees.Add(r.worth(a.Base, a.Time))
			}
		case r.external(a):
			t.Realized = t.Realized.Add(r.worth(a.Base, a.Time)).Sub(taken)
		}
		feeOn = t
	}
	if a.Fee == nil || a.Fee.Qty.IsZero() {
		return
	}
	v := r.worth(*a.Fee, a.Time)
	r.Fees = r.Fees.Add(v)
	if feeOn != nil {
		feeOn.Fees = feeOn.Fees.Add(v)
	}
	isTrade := a.Type == model.Trade && a.Quote != nil
	netted := isTrade && (a.Fee.Asset.Key() == a.Base.Asset.Key() && a.Side == model.Buy ||
		a.Fee.Asset.Key() == a.Quote.Asset.Key() && a.Side == model.Sell)
	if netted {
		// Already out of what was received.
		r.move(a.SourceID, *a.Fee, -1, a.Time)
		return
	}
	ft := r.move(a.SourceID, *a.Fee, -1, a.Time)
	if ft != nil {
		addCost(ft, r.book.Fees[a.ID].Neg())
		if !isTrade {
			ft.Realized = ft.Realized.Sub(r.book.Fees[a.ID])
		}
	}
}

// run replays everything up to at.
func (r *replay) run(l *Ledger, at time.Time) {
	for _, a := range model.ReplayOrder(l.Acts, l.Pairs) {
		if a.Time.After(at) {
			break
		}
		r.apply(a)
	}
}
