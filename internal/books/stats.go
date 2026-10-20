package books

// The statistics the pages draw: the value day by day with its
// time-weighted index and each period's change (overview), month by month
// figures and each coin's results (stats), and one coin in detail (asset).

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/perf"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/Alexis-Marcel/fundarr/pkg/portfolio"
	"github.com/shopspring/decimal"
)

const dayLen = 24 * time.Hour

// series is the portfolio day by day (a week when that is too many days).
type series struct {
	version int64
	at      time.Time
	step    string
	days    []perf.Day
}

func f64(d decimal.Decimal) float64 { v, _ := d.Float64(); return v }

// r2 rounds for the page: cents.
func r2(v float64) float64 { return math.Round(v*100) / 100 }

// sig keeps 6 significant digits (prices of coins worth little).
func sig(v float64) float64 {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	p := math.Pow(10, 5-math.Floor(math.Log10(math.Abs(v))))
	return math.Round(v*p) / p
}

// r6 rounds a ratio.
func r6(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// series computes the history from the first operation to today, kept
// while nothing changes (the caller holds e.mu).
func (e *Engine) series(l *Ledger, p *pricer, scope string) *series {
	now := e.Now()
	if s := e.ser[scope]; s != nil && s.version == l.Version && now.Sub(s.at) < e.TTL {
		return s
	}
	today := now.Truncate(dayLen)
	s := &series{version: l.Version, at: now, step: "day"}
	m := p.money(l)
	start := portfolio.First(l.Acts)
	if f := m.First(scope); !f.IsZero() && (start.IsZero() || f.Before(start)) {
		start = f
	}
	if !start.IsZero() && !start.After(today) {
		days := portfolio.Days(start, today, "day")
		if days == nil {
			s.step = "week"
			days = portfolio.Days(start, today, "week")
		}
		// The coins and securities held, and the cash beside them; the
		// money invested is every external flow.
		points, _ := portfolio.History(l.Acts, l.Pairs, p.memoize, days)
		cash := m.Days(scope, p.memoize, days)
		for n, pt := range points {
			c := cash[n]
			s.days = append(s.days, perf.Day{T: pt.Day, Value: f64(pt.Value.Add(c.Cash)), Invested: f64(c.Invested),
				In: f64(c.Day.In), Out: f64(c.Day.Out), Adj: f64(c.Day.Adj)})
		}
	}
	if !p.late.Load() {
		e.ser[scope] = s
	}
	return s
}

// Periods offered, and where each starts (the eve of its first day).
var Periods = []string{"7J", "1M", "YTD", "1A", "Tout"}

func periodStart(key string, today time.Time) time.Time {
	switch key {
	case "7J":
		return today.AddDate(0, 0, -7)
	case "1M":
		return today.AddDate(0, -1, 0)
	case "YTD":
		return time.Date(today.Year()-1, 12, 31, 0, 0, 0, 0, time.UTC)
	case "1A":
		return today.AddDate(-1, 0, 0)
	}
	return time.Time{}
}

// Period is a period's change: Gain is what the value did beyond the
// money put in or taken out; TWR the time-weighted return.
type Period struct {
	Key string `json:"key"`
	// From: index of the period's opening point (its eve) in the points.
	From  int     `json:"from"`
	Start float64 `json:"start"` // value at the opening
	End   float64 `json:"end"`
	Gain  float64 `json:"gain"`
	// Pct: the gain over what was at stake (perf.Simple), nil when nothing was.
	Pct *float64 `json:"pct"`
	TWR float64  `json:"twr"`
}

// periods measures each period over days.
func periods(days []perf.Day, today time.Time) []Period {
	out := []Period{}
	if len(days) == 0 {
		return out
	}
	f := perf.Factors(days)
	last := days[len(days)-1]
	for _, key := range Periods {
		at := periodStart(key, today)
		// The opening: the last point on or before the eve; none for "Tout"
		// or when the history starts later (it starts from nothing).
		from := -1
		if !at.IsZero() {
			from = sort.Search(len(days), func(i int) bool { return days[i].T.After(at) }) - 1
		}
		p := Period{Key: key, From: max(from, 0), End: r2(last.Value)}
		openV, openI := 0.0, 0.0
		if from >= 0 {
			openV, openI = days[from].Value, days[from].Invested
		}
		p.Start = r2(openV)
		p.Gain = r2(last.Value - last.Invested - (openV - openI))
		p.TWR = r6(perf.TWR(f, from+1, len(f)))
		if pct, ok := perf.Simple(openV, openI, last.Value, last.Invested); ok {
			pct = r6(pct)
			p.Pct = &pct
		}
		out = append(out, p)
	}
	return out
}

// Overview is GET /api/overview: the value day by day with the money put
// in and the time-weighted index, each period's change, and the totals.
func (e *Engine) Overview(ctx context.Context, scope string) ([]byte, error) {
	scope, err := ParseScope(scope)
	if err != nil {
		return nil, err
	}
	return e.cached(ctx, "overview?"+scope, func(l *Ledger, p *pricer) (any, error) {
		l = p.scoped(l, scope)
		at := e.Now()
		today := at.Truncate(dayLen)
		s := e.series(l, p, scope)
		sum := portfolio.Now(l.Acts, l.Pairs, p.memoize, at)
		r := newReplay(l, p.memoize)
		r.run(l, at)
		m := p.money(l)
		_, cash, flows := m.Now(scope, p.memoize, at)

		type point struct {
			D string  `json:"d"`
			V float64 `json:"v"` // value
			I float64 `json:"i"` // invested
			X float64 `json:"x"` // time-weighted index, 1 at the start
		}
		pts := make([]point, len(s.days))
		x := perf.Index(perf.Factors(s.days))
		for n, d := range s.days {
			pts[n] = point{d.T.Format(time.DateOnly), r2(d.Value), r2(d.Invested), r6(x[n])}
		}
		// Now's value, cash included, against every external flow.
		value, invested := f64(sum.Value.Add(cash)), f64(flows.Invested())
		var xirr *float64
		if rate, ok := perf.XIRR(perf.Flows(s.days, 0)); ok && len(s.days) > 0 {
			rate = r6(rate)
			xirr = &rate
		}
		stable := decimal.Zero
		classes := map[string]float64{}
		for _, h := range sum.Assets {
			if model.CashLike(h.Asset) {
				stable = stable.Add(h.Value)
			}
			classes[Class(h.Asset)] += f64(h.Value)
		}
		if cash.IsPositive() {
			classes[ClassCash] += f64(cash)
		}
		for k, v := range classes {
			classes[k] = r2(v)
		}
		return map[string]any{
			"currency": Currency,
			"at":       at,
			"step":     s.step,
			"points":   pts,
			"periods":  periods(s.days, today),
			"total": map[string]any{
				"value": r2(value), "invested": r2(invested), "gain": r2(value - invested),
				"realized": r2(f64(sum.Realized)), "unrealized": r2(f64(sum.Unrealized)),
				"rewards": r2(f64(sum.Rewards)), "dividends": r2(f64(r.Dividends)), "interest": r2(f64(r.Interest)),
				"fees": r2(f64(r.Fees)), "stable": r2(f64(stable)), "xirr": xirr,
				// Money put in and taken out, the corrections apart (in
				// invested, not here), and the cash held.
				"paidIn": r2(f64(flows.In)), "paidOut": r2(f64(flows.Out)), "contributed": r2(f64(flows.Net())),
				"adjusted": r2(f64(flows.Adj)), "cash": r2(f64(cash)),
			},
			"classes":  classes,
			"unpriced": assets(append(sum.Unpriced, m.unpricedCash(scope, p.memoize, at)...)),
		}, nil
	})
}

// coinView is one coin's results.
type coinView struct {
	Asset      model.Asset `json:"asset"`
	Qty        string      `json:"qty"`
	Price      float64     `json:"price"`
	Value      float64     `json:"value"`
	Cost       float64     `json:"cost"`
	AvgCost    float64     `json:"avgCost"`
	Share      float64     `json:"share"`
	Unrealized float64     `json:"unrealized"`
	Realized   float64     `json:"realized"`
	Rewards    float64     `json:"rewards"`
	Dividends  float64     `json:"dividends"`
	Fees       float64     `json:"fees"`
	// Gain: unrealized, realized, rewards and dividends; Pct: of what was
	// bought.
	Gain   float64  `json:"gain"`
	Pct    *float64 `json:"pct,omitempty"`
	Held   bool     `json:"held"`
	Stable bool     `json:"stable,omitempty"`
	// ID: how links name it (ISIN or symbol); Class: crypto, stable, stock, etf.
	ID    string    `json:"id"`
	Class string    `json:"class"`
	First string    `json:"first"`
	Spark []float64 `json:"spark,omitempty"`
}

func coinOf(t *tally, h *portfolio.Holding, total decimal.Decimal) coinView {
	v := coinView{Asset: t.Asset, Qty: "0", Realized: r2(f64(t.Realized)), Rewards: r2(f64(t.Rewards)), Dividends: r2(f64(t.Dividends)),
		Fees: r2(f64(t.Fees)), Stable: model.CashLike(t.Asset), First: t.First.UTC().Format(time.DateOnly),
		ID: AssetID(t.Asset), Class: Class(t.Asset)}
	if h != nil {
		v.Qty, v.Price, v.Value, v.Cost = h.Qty.String(), f64(h.Price), r2(f64(h.Value)), r2(f64(h.Cost))
		v.Unrealized, v.Held = r2(f64(h.Unrealized)), true
		if total.IsPositive() {
			v.Share = r6(f64(h.Value.Div(total)))
		}
		if h.Qty.IsPositive() {
			v.AvgCost = sig(f64(h.Cost.Div(h.Qty)))
		}
	}
	v.Gain = r2(v.Unrealized + v.Realized + v.Rewards + v.Dividends)
	if b := f64(t.Bought); b > 0.5 && !v.Stable {
		pct := r6(v.Gain / b)
		v.Pct = &pct
	}
	return v
}

// coins: every crypto and security ever held, held ones first by value;
// shares are of total (cash included).
func coins(r *replay, sum portfolio.Summary, total decimal.Decimal) []coinView {
	held := map[string]*portfolio.Holding{}
	for i := range sum.Assets {
		held[sum.Assets[i].Asset.Key()] = &sum.Assets[i]
	}
	out := []coinView{}
	for k, t := range r.coins {
		out = append(out, coinOf(t, held[k], total))
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Held != b.Held {
			return a.Held
		}
		if a.Value != b.Value {
			return a.Value > b.Value
		}
		if a.Gain != b.Gain {
			return a.Gain > b.Gain
		}
		return a.Asset.Symbol < b.Asset.Symbol
	})
	return out
}

// Stats is GET /api/stats: month by month (returns, money in and out,
// rewards, value and invested), and each coin's results.
func (e *Engine) Stats(ctx context.Context, scope string) ([]byte, error) {
	scope, err := ParseScope(scope)
	if err != nil {
		return nil, err
	}
	return e.cached(ctx, "stats?"+scope, func(l *Ledger, p *pricer) (any, error) {
		l = p.scoped(l, scope)
		at := e.Now()
		today := at.Truncate(dayLen)
		s := e.series(l, p, scope)
		sum := portfolio.Now(l.Acts, l.Pairs, p.memoize, at)
		r := newReplay(l, p.memoize)
		r.run(l, at)
		cashList, cash, _ := p.money(l).Now(scope, p.memoize, at)

		type month struct {
			M      string  `json:"m"`
			Return float64 `json:"ret"`
			// Money put in and taken out (≥ 0); Adj: the corrections.
			In  float64 `json:"in"`
			Out float64 `json:"out"`
			Adj float64 `json:"adj"`
			// The month's rewards (staking), dividends (net of tax) and
			// interest on cash.
			Rewards   float64 `json:"rewards"`
			Dividends float64 `json:"dividends"`
			Interest  float64 `json:"interest"`
			Value     float64 `json:"value"`
			// Invested: every external flow so far, the corrections
			// among them (Adjusted).
			Invested float64 `json:"invested"`
			Adjusted float64 `json:"adjusted"`
			// Earned: rewards, dividends and interest so far, at their value
			// when received.
			Earned float64 `json:"earned"`
		}
		type sums struct{ rewards, dividends, interest float64 }
		byMonth := map[string]*sums{}
		for d, v := range r.Income {
			m := byMonth[d[:7]]
			if m == nil {
				m = &sums{}
				byMonth[d[:7]] = m
			}
			m.rewards += f64(v.Rewards)
			m.dividends += f64(v.Dividends)
			m.interest += f64(v.Interest)
		}
		months := []month{}
		earned, adjusted := 0.0, 0.0
		for _, m := range perf.Monthly(s.days) {
			x := byMonth[m.Month]
			if x == nil {
				x = &sums{}
			}
			earned += x.rewards + x.dividends + x.interest
			adjusted += m.Adj
			months = append(months, month{m.Month, r6(m.Return), r2(m.In), r2(m.Out), r2(m.Adj), r2(x.rewards), r2(x.dividends), r2(x.interest),
				r2(m.Value), r2(m.Invested), r2(adjusted), r2(earned)})
		}

		list := coins(r, sum, sum.Value.Add(cash))
		// Sparklines: the last 30 days' closes of what is held.
		for i := range list {
			if !list[i].Held || list[i].Stable {
				continue
			}
			for d := today.AddDate(0, 0, -29); !d.After(today); d = d.Add(dayLen) {
				if u, ok := p.memoize(list[i].Asset, d); ok {
					list[i].Spark = append(list[i].Spark, sig(f64(u)))
				}
			}
		}
		return map[string]any{"currency": Currency, "at": at, "months": months, "assets": list,
			"cash": cashView(l, cashList, sum.Value.Add(cash))}, nil
	})
}

// Asset is GET /api/assets/{symbol}: one coin's results, and day by day
// its price, the quantity held and its average cost, with each buy and sale.
func (e *Engine) Asset(ctx context.Context, symbol string) ([]byte, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	return e.cached(ctx, "asset?"+symbol, func(l *Ledger, p *pricer) (any, error) {
		at := e.Now()
		today := at.Truncate(dayLen)
		asset, known := l.Find(symbol)
		if !known {
			return nil, &Error{404, "Aucune opération en " + symbol}
		}
		key := asset.Key()
		sum := portfolio.Now(l.Acts, l.Pairs, p.memoize, at)
		r := newReplay(l, p.memoize)
		order := model.ReplayOrder(l.Acts, l.Pairs)

		type point struct {
			D   string  `json:"d"`
			P   float64 `json:"p"`             // close
			Q   float64 `json:"q"`             // held
			Avg float64 `json:"avg,omitempty"` // average cost
		}
		type marker struct {
			D     string  `json:"d"`
			Buy   bool    `json:"buy"`
			Qty   string  `json:"qty"`
			Value float64 `json:"value"`
			Price float64 `json:"price"`
		}
		var pts []point
		i := 0
		var start time.Time
		for _, a := range order {
			if a.Base.Asset.Key() == key || a.Quote != nil && a.Quote.Asset.Key() == key {
				start = a.Time.UTC().Truncate(dayLen)
				break
			}
		}
		step := "day"
		days := portfolio.Days(start, today, step)
		if days == nil {
			step = "week"
			days = portfolio.Days(start, today, step)
		}
		for _, d := range days {
			end := d.Add(dayLen)
			for ; i < len(order) && order[i].Time.Before(end); i++ {
				r.apply(order[i])
			}
			pt := point{D: d.Format(time.DateOnly)}
			if u, ok := p.memoize(asset, d); ok {
				pt.P = sig(f64(u))
			}
			if t := r.coins[key]; t != nil {
				q := t.Qty()
				pt.Q = sig(f64(q))
				if q.IsPositive() && !model.CashLike(asset) {
					pt.Avg = sig(f64(t.Cost.Div(q)))
				}
			}
			pts = append(pts, pt)
		}
		for ; i < len(order) && !order[i].Time.After(at); i++ {
			r.apply(order[i])
		}
		marks := []marker{}
		for _, m := range r.Markers {
			if m.Asset != key || m.Qty.IsZero() {
				continue
			}
			marks = append(marks, marker{m.Time.UTC().Format(time.DateOnly), m.Buy, m.Qty.String(), r2(f64(m.Value)),
				sig(f64(m.Value.Div(m.Qty)))})
		}
		_, cash, _ := p.money(l).Now(ScopeAll, p.memoize, at)
		var view coinView
		for _, c := range coins(r, sum, sum.Value.Add(cash)) {
			if c.Asset.Key() == key {
				view = c
			}
		}
		if pts == nil {
			pts = []point{}
		}
		return map[string]any{"currency": Currency, "at": at, "step": step, "coin": view, "points": pts, "markers": marks}, nil
	})
}

// cashAccount is an account's cash, for the pages: its value in euros and
// each currency held.
type cashAccount struct {
	Account Ref     `json:"account"`
	Value   float64 `json:"value"`
	Share   float64 `json:"share"`
	Held    []struct {
		Symbol string `json:"symbol"`
		Qty    string `json:"qty"`
	} `json:"held"`
}

// cashView groups cash by account, largest first; shares are of total.
func cashView(l *Ledger, list []Cash, total decimal.Decimal) []cashAccount {
	out := []cashAccount{}
	at := map[string]int{}
	for _, c := range list {
		n, ok := at[c.Acc]
		if !ok {
			n = len(out)
			at[c.Acc] = n
			out = append(out, cashAccount{Account: l.ref(c.Acc)})
		}
		a := &out[n]
		a.Value += f64(c.Value)
		a.Held = append(a.Held, struct {
			Symbol string `json:"symbol"`
			Qty    string `json:"qty"`
		}{c.Cur.Symbol, c.Qty.StringFixed(2)})
	}
	for i := range out {
		if total.IsPositive() {
			out[i].Share = r6(out[i].Value / f64(total))
		}
		out[i].Value = r2(out[i].Value)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out
}
