package server

// The strategy: the shares aimed at, the monthly plan, the goal and the
// rules the Stratégie page checks. Kept as one setting; the page computes
// the rest from the portfolio.

import (
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/books"
)

// Plan is GET and PUT /api/plan. Shares are whole percents.
type Plan struct {
	// Set: the plan was saved once (otherwise these are the defaults).
	Set bool `json:"set"`
	// Crypto: of the invested whole (cash apart); BTC and ETH: of the
	// crypto, the rest going to the others; ETF: of stocks and ETFs.
	Crypto int `json:"crypto"`
	BTC    int `json:"btc"`
	ETH    int `json:"eth"`
	ETF    int `json:"etf"`
	// Band: how far, in points, crypto may drift before an alert; Line: the
	// share one stock or small coin may reach.
	Band int `json:"band"`
	Line int `json:"line"`
	// Monthly: euros put in each month, since Since (YYYY-MM, set when the
	// amount changes).
	Monthly int    `json:"monthly"`
	Since   string `json:"since,omitempty"`
	// Goal: euros, by GoalYear.
	Goal     int `json:"goal"`
	GoalYear int `json:"goalYear"`
	// PEA: the day the PEA was opened (YYYY-MM-DD).
	PEA string `json:"pea,omitempty"`
	// Custody: the accounts that are the owner's own wallets; nil: guessed
	// from their names.
	Custody []string `json:"custody"`
}

const planKey = "plan"

var defaultPlan = Plan{Crypto: 20, BTC: 70, ETH: 20, ETF: 80, Band: 10, Line: 15}

var idRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

func (s *Server) plan(r *http.Request) (Plan, error) {
	raw, err := s.St.Setting(r.Context(), planKey)
	if err != nil || raw == "" {
		return defaultPlan, err
	}
	p := defaultPlan
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return defaultPlan, nil
	}
	p.Set = true
	return p, nil
}

func (s *Server) getPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.plan(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, p)
}

func (p Plan) check() string {
	in := func(v, lo, hi int) bool { return v >= lo && v <= hi }
	switch {
	case !in(p.Crypto, 0, 100) || !in(p.BTC, 0, 100) || !in(p.ETH, 0, 100) || !in(p.ETF, 0, 100):
		return "Parts : de 0 à 100 %"
	case p.BTC+p.ETH > 100:
		return "Bitcoin et Ether : 100 % au plus à eux deux"
	case !in(p.Band, 1, 50):
		return "Écart toléré : de 1 à 50 points"
	case !in(p.Line, 1, 100):
		return "Plus grosse ligne : de 1 à 100 %"
	case !in(p.Monthly, 0, 10_000_000) || !in(p.Goal, 0, 1_000_000_000):
		return "Montant invalide"
	case p.Goal > 0 && !in(p.GoalYear, 2000, 2200):
		return "Année de l'objectif invalide"
	case len(p.Custody) > 500:
		return "Trop de comptes"
	}
	if p.PEA != "" {
		if _, err := time.Parse(time.DateOnly, p.PEA); err != nil {
			return "Date du PEA : AAAA-MM-JJ"
		}
	}
	for _, id := range p.Custody {
		if !idRe.MatchString(id) {
			return "Compte invalide"
		}
	}
	return ""
}

func (s *Server) putPlan(w http.ResponseWriter, r *http.Request) {
	var p Plan
	if !decode(w, r, 64<<10, &p) {
		return
	}
	if msg := p.check(); msg != "" {
		s.fail(w, r, &books.Error{Status: 400, Msg: msg})
		return
	}
	old, err := s.plan(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// The plan is followed from the month its amount was set.
	switch {
	case p.Monthly == 0:
		p.Since = ""
	case p.Monthly != old.Monthly || old.Since == "":
		p.Since = s.now().Format("2006-01")
	default:
		p.Since = old.Since
	}
	if p.Goal == 0 {
		p.GoalYear = 0
	}
	p.Set = false
	raw, _ := json.Marshal(p)
	if err := s.St.Set(r.Context(), map[string]string{planKey: string(raw)}); err != nil {
		s.fail(w, r, err)
		return
	}
	p.Set = true
	writeJSON(w, 200, p)
}
