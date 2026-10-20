// Package fake is a stand-in for Fundarr's read API, for developing Cairn
// without a Fundarr: a few years of a made-up but consistent portfolio
// (monthly buys, staking, moves to a wallet, swaps, sales for euros).
//
// Everything comes from one replay of the same activities, so the
// portfolio, its history and the tax lines agree with each other the way
// Fundarr's do. Prices are a deterministic random walk pinned to plausible
// levels, the same on every run.
package fake

import (
	"crypto/sha1"
	"encoding/hex"
	"math"
	"math/rand/v2"
	"sort"
	"time"
)

// Asset, Amount and the types below are the shapes of docs/api.md.
type Asset struct {
	Kind   string `json:"kind"`
	Symbol string `json:"symbol"`
}

type Amount struct {
	Asset Asset  `json:"asset"`
	Qty   string `json:"qty"`
}

type SourceRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

type Activity struct {
	ID           string    `json:"id"`
	Source       SourceRef `json:"source"`
	Account      string    `json:"account"`
	Time         time.Time `json:"time"`
	Type         string    `json:"type"`
	Side         string    `json:"side,omitempty"`
	Base         *Amount   `json:"base,omitempty"`
	Quote        *Amount   `json:"quote,omitempty"`
	Fee          *Amount   `json:"fee,omitempty"`
	Cost         string    `json:"cost,omitempty"`
	Currency     string    `json:"currency,omitempty"`
	TransferWith string    `json:"transferWith,omitempty"`
}

var (
	binance = SourceRef{ID: "9b687b4899f5", Name: "Binance", Kind: "binance"}
	okx     = SourceRef{ID: "01a3c5e7f9b2", Name: "OKX", Kind: "okx"}
	ledger  = SourceRef{ID: "0130c2d4e6f8", Name: "Ledger", Kind: "wallet"}
	sources = []SourceRef{binance, okx, ledger}
)

// start is the first day of prices; activities begin a few weeks later.
var start = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

// unpricedSym is an airdropped token no price source knows.
const unpricedSym = "PYTH"

// waypoint pins an asset's price (EUR) on a day; the walk goes through it.
type waypoint struct {
	day   string
	price float64
}

type market struct {
	vol    float64 // daily volatility of the walk between waypoints
	points []waypoint
}

var markets = map[string]market{
	"BTC":  {0.028, []waypoint{{"2023-01-01", 15500}, {"2023-06-15", 24000}, {"2023-10-15", 25500}, {"2024-03-12", 64000}, {"2024-08-05", 49000}, {"2024-12-17", 102000}, {"2025-04-08", 71000}, {"2025-08-14", 106000}, {"2025-11-20", 78000}, {"2026-03-01", 72000}, {"2026-07-01", 88000}, {"", 95000}}},
	"ETH":  {0.035, []waypoint{{"2023-01-01", 1120}, {"2023-04-15", 1900}, {"2023-10-15", 1450}, {"2024-03-12", 3700}, {"2024-08-05", 2200}, {"2024-12-17", 3800}, {"2025-04-08", 1400}, {"2025-08-22", 4000}, {"2025-11-20", 2600}, {"2026-03-01", 2200}, {"", 3300}}},
	"SOL":  {0.05, []waypoint{{"2023-01-01", 9}, {"2023-06-15", 14}, {"2023-12-25", 100}, {"2024-03-18", 180}, {"2024-08-05", 125}, {"2024-11-22", 240}, {"2025-04-08", 95}, {"2025-09-18", 210}, {"2025-11-20", 125}, {"", 160}}},
	"ADA":  {0.04, []waypoint{{"2023-01-01", 0.24}, {"2023-04-15", 0.42}, {"2023-10-15", 0.24}, {"2024-03-12", 0.70}, {"2024-08-05", 0.30}, {"2024-12-03", 1.05}, {"2025-04-08", 0.55}, {"2025-11-20", 0.42}, {"", 0.55}}},
	"USDT": {0.002, []waypoint{{"2023-01-01", 0.94}, {"2024-01-01", 0.91}, {"2025-01-01", 0.96}, {"2025-07-01", 0.86}, {"", 0.88}}},
}

// Point is one day of history.
type Point struct {
	Date     string
	Value    float64
	Invested float64
}

// Disposal is one sale for euros, with its 2086 line in both modes.
type Disposal struct {
	Time      time.Time
	Source    SourceRef
	Sym       string
	Qty       float64
	Price     float64 // received, before the fees
	Fees      float64
	Portfolio float64
	Acq       [2]float64 // acquisition: [0] plain, [1] rewards=income
	Share     [2]float64
	Gain      [2]float64
}

// Holding is what the replay ends with for one asset.
type Holding struct {
	Sym      string
	Qty      float64
	Cost     float64
	BySource map[string]float64
}

// Book is the whole made-up history.
type Book struct {
	Today      time.Time
	Activities []Activity
	History    []Point
	Disposals  []Disposal
	Holdings   []Holding
	Prices     map[string]float64 // today's
	Invested   float64
	Realized   float64
	Rewards    float64

	unpricedSince string // the day the unpriced token came
}

// prices is the close of each asset on each day since start.
type prices map[string][]float64

func walk(today time.Time) prices {
	days := int(today.Sub(start).Hours()/24) + 1
	out := prices{}
	syms := make([]string, 0, len(markets))
	for s := range markets {
		syms = append(syms, s)
	}
	sort.Strings(syms)
	for i, s := range syms {
		m := markets[s]
		rng := rand.New(rand.NewPCG(42, uint64(i)))
		noise := make([]float64, days)
		for d := 1; d < days; d++ {
			noise[d] = noise[d-1] + rng.NormFloat64()*m.vol
		}
		// Waypoints as day indexes; "" is today.
		idx := make([]int, len(m.points))
		for k, w := range m.points {
			if w.day == "" {
				idx[k] = days - 1
			} else {
				t, _ := time.Parse(time.DateOnly, w.day)
				idx[k] = min(int(t.Sub(start).Hours()/24), days-1)
			}
		}
		p := make([]float64, days)
		for k := 0; k+1 < len(idx); k++ {
			a, b := idx[k], idx[k+1]
			if b <= a {
				continue
			}
			la, lb := math.Log(m.points[k].price), math.Log(m.points[k+1].price)
			for d := a; d <= b; d++ {
				f := float64(d-a) / float64(b-a)
				// A bridge: the walk minus its straight line, so it is 0 at both ends.
				bridge := noise[d] - (noise[a] + (noise[b]-noise[a])*f)
				p[d] = math.Exp(la + (lb-la)*f + bridge)
			}
		}
		out[s] = p
	}
	return out
}

// sim replays the activities.
type sim struct {
	px      prices
	day     int
	hold    map[string]map[string]float64 // source id → symbol → qty
	basis   map[string]float64            // symbol → average cost of what is held
	acts    []Activity
	b       *Book
	acqFiat float64    // fiat paid for crypto, fees included
	acqFree float64    // rewards and outside deposits, at their value
	used    [2]float64 // capital shares already used, per mode
}

func (s *sim) price(sym string) float64 {
	if p, ok := s.px[sym]; ok {
		return p[s.day]
	}
	return 0
}

func (s *sim) qty(sym string) float64 {
	t := 0.0
	for _, h := range s.hold {
		t += h[sym]
	}
	return t
}

func (s *sim) value() float64 {
	v := 0.0
	for _, h := range s.hold {
		for sym, q := range h {
			v += q * s.price(sym)
		}
	}
	return v
}

func (s *sim) at(hour, minute int) time.Time {
	return start.AddDate(0, 0, s.day).Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
}

func (s *sim) add(a Activity) string {
	sum := sha1.Sum([]byte(a.Time.String() + a.Type + a.Source.ID + func() string {
		if a.Base != nil {
			return a.Base.Asset.Symbol + a.Base.Qty
		}
		return ""
	}()))
	a.ID = hex.EncodeToString(sum[:])[:24]
	s.acts = append(s.acts, a)
	return a.ID
}

func crypto(sym string, q float64) *Amount {
	return &Amount{Asset{"crypto", sym}, qtyStr(q)}
}

func eur(q float64) *Amount { return &Amount{Asset{"fiat", "EUR"}, money(q)} }

// removeBasis takes the average cost of qty out of sym's basis.
func (s *sim) removeBasis(sym string, qty float64) float64 {
	total := s.qty(sym)
	if total <= 0 {
		return 0
	}
	part := s.basis[sym] * qty / total
	s.basis[sym] -= part
	return part
}

func (s *sim) buy(src SourceRef, sym string, spent float64) {
	fee := spent * 0.001
	q := (spent - fee) / s.price(sym)
	s.hold[src.ID][sym] += q
	s.basis[sym] += spent
	s.b.Invested += spent
	s.acqFiat += spent
	s.add(Activity{Source: src, Time: s.at(9, 12), Type: "trade", Side: "buy", Base: crypto(sym, q), Quote: eur(spent - fee), Fee: eur(fee), Cost: money(spent), Currency: "EUR"})
}

// sell sells part (0 to 1) of what src holds of sym, for euros.
func (s *sim) sell(src SourceRef, sym string, part float64) {
	q := s.hold[src.ID][sym] * part
	if q <= 0 {
		return
	}
	s.sellQty(src, sym, q)
}

func (s *sim) sellQty(src SourceRef, sym string, q float64) {
	gross := q * s.price(sym)
	fee := gross * 0.001
	net := gross - fee
	portfolio := s.value()
	cost := s.removeBasis(sym, q)
	s.hold[src.ID][sym] -= q
	s.b.Realized += net - cost
	s.b.Invested -= net

	// Line 224: the fees reduce the gain, not the ratio.
	d := Disposal{Time: s.at(14, 37), Source: src, Sym: sym, Qty: q, Price: gross, Fees: fee, Portfolio: portfolio}
	for m := range 2 {
		acq := s.acqFiat - s.used[m]
		if m == 1 {
			acq += s.acqFree
		}
		share := acq * gross / portfolio
		s.used[m] += share
		d.Acq[m], d.Share[m], d.Gain[m] = acq, share, gross-fee-share
	}
	s.b.Disposals = append(s.b.Disposals, d)
	s.add(Activity{Source: src, Time: d.Time, Type: "trade", Side: "sell", Base: crypto(sym, q), Quote: eur(net), Fee: eur(fee), Currency: "EUR"})
}

// stable are the stablecoins: selling for them realizes a gain.
var stable = map[string]bool{"USDT": true, "USDC": true}

// swap exchanges part of src's from for to: not a taxable event. For a
// stablecoin the gain is realized; for another coin the cost carries over.
func (s *sim) swap(src SourceRef, from string, part float64, to string) {
	q := s.hold[src.ID][from] * part
	v := q * s.price(from)
	cost := s.removeBasis(from, q)
	s.hold[src.ID][from] -= q
	got := v * 0.999 / s.price(to)
	s.hold[src.ID][to] += got
	if stable[to] {
		s.b.Realized += v - cost
		s.basis[to] += v
	} else {
		s.basis[to] += cost
	}
	s.add(Activity{Source: src, Time: s.at(16, 4), Type: "trade", Side: "buy", Base: crypto(to, got), Quote: crypto(from, q), Cost: money(v), Currency: "EUR"})
}

// move sends part of from's sym to another of my sources.
func (s *sim) move(from, to SourceRef, sym string, part, fee float64) {
	q := s.hold[from.ID][sym] * part
	if q <= fee {
		return
	}
	s.hold[from.ID][sym] -= q
	s.hold[to.ID][sym] += q - fee
	t := s.at(11, 30)
	out := s.add(Activity{Source: from, Time: t, Type: "withdrawal", Base: crypto(sym, q-fee), Fee: crypto(sym, fee), TransferWith: to.ID})
	s.add(Activity{Source: to, Time: t.Add(20 * time.Minute), Type: "deposit", Base: crypto(sym, q-fee), Cost: money(s.basis[sym] * (q - fee) / s.qty(sym)), Currency: "EUR", TransferWith: out})
}

// stake pays src a reward of rate × what it holds of sym.
func (s *sim) stake(src SourceRef, sym string, rate float64) {
	q := s.hold[src.ID][sym] * rate
	if q <= 0 {
		return
	}
	v := q * s.price(sym)
	s.hold[src.ID][sym] += q
	s.basis[sym] += v
	s.b.Rewards += v
	s.acqFree += v
	s.add(Activity{Source: src, Time: s.at(2, 0), Type: "reward", Base: crypto(sym, q), Cost: money(v), Currency: "EUR"})
}

// deposit is crypto coming from outside (an old wallet): invested.
func (s *sim) deposit(src SourceRef, sym string, q float64) {
	v := q * s.price(sym)
	s.hold[src.ID][sym] += q
	s.basis[sym] += v
	s.b.Invested += v
	s.acqFree += v
	s.add(Activity{Source: src, Time: s.at(18, 45), Type: "deposit", Base: crypto(sym, q), Cost: money(v), Currency: "EUR"})
}

// Generate replays everything from 2023 to today.
func Generate(today time.Time) *Book {
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	b := &Book{Today: today, Prices: map[string]float64{}}
	s := &sim{px: walk(today), hold: map[string]map[string]float64{}, basis: map[string]float64{}, b: b}
	for _, src := range sources {
		s.hold[src.ID] = map[string]float64{}
	}
	days := len(s.px["BTC"])
	for s.day = 0; s.day < days; s.day++ {
		t := start.AddDate(0, 0, s.day)
		y, m, d := t.Date()
		ym := y*100 + int(m)
		on := func(date string) bool { return t.Format(time.DateOnly) == date }

		// Staking pays on the 1st.
		if d == 1 {
			s.stake(binance, "ETH", 0.0031)
			s.stake(okx, "SOL", 0.0052)
		}
		// Monthly buys.
		if d == 5 && ym >= 202302 {
			s.buy(binance, "BTC", map[bool]float64{true: 250, false: 200}[y >= 2025])
			s.buy(binance, "ETH", 100)
		}
		if d == 8 && ym >= 202306 {
			s.buy(okx, "SOL", 60)
		}
		// Coins go to the wallet twice a year.
		if (m == 1 || m == 7) && d == 2 && ym > 202302 {
			s.move(binance, ledger, "BTC", 0.95, 0.00002)
		}
		switch {
		case on("2023-03-20"):
			s.buy(binance, "ETH", 1500)
		case on("2023-09-12"):
			s.deposit(ledger, "ETH", 0.6)
		case on("2023-11-20"):
			s.sellQty(okx, "SOL", 250/s.price("SOL")) // under 305 €: the year is exempt
		case on("2024-01-15"):
			s.move(binance, ledger, "ETH", 0.5, 0.0004)
		case on("2024-02-10"):
			s.swap(binance, "ETH", 0.3, "ADA")
		case on("2024-03-14"):
			s.sell(binance, "ETH", 0.25)
		case on("2024-11-12"):
			s.buy(binance, "BTC", 2000)
		case on("2024-12-09"):
			s.sell(binance, "BTC", 0.4)
		case on("2025-01-20"):
			s.sell(binance, "ADA", 1)
		case on("2025-03-01"):
			s.swap(binance, "ETH", 0.3, "USDT")
		case on("2025-06-10"):
			s.sell(binance, "USDT", 0.5)
		case on("2025-08-02"):
			s.buy(binance, "ETH", 1000)
		case on("2025-11-15"):
			s.sell(okx, "SOL", 0.4)
		case on("2026-02-16"):
			s.hold[ledger.ID][unpricedSym] += 420
			b.unpricedSince = t.Format(time.DateOnly)
			s.add(Activity{Source: ledger, Time: s.at(7, 0), Type: "reward", Base: crypto(unpricedSym, 420)})
		case on("2026-05-04"):
			s.sell(binance, "ETH", 0.2)
		}
		b.History = append(b.History, Point{Date: t.Format(time.DateOnly), Value: s.value(), Invested: b.Invested})
	}
	s.day = days - 1

	for sym := range markets {
		b.Prices[sym] = s.price(sym)
	}
	syms := map[string]bool{}
	for _, h := range s.hold {
		for sym, q := range h {
			if q > 1e-12 {
				syms[sym] = true
			}
		}
	}
	for sym := range syms {
		h := Holding{Sym: sym, Qty: s.qty(sym), Cost: s.basis[sym], BySource: map[string]float64{}}
		for id, hs := range s.hold {
			if hs[sym] > 1e-12 {
				h.BySource[id] = hs[sym]
			}
		}
		b.Holdings = append(b.Holdings, h)
	}
	sort.SliceStable(s.acts, func(i, j int) bool {
		if !s.acts[i].Time.Equal(s.acts[j].Time) {
			return s.acts[i].Time.Before(s.acts[j].Time)
		}
		return s.acts[i].ID < s.acts[j].ID
	})
	b.Activities = s.acts
	return b
}
