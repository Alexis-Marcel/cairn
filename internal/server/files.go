package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/books"
	"github.com/Alexis-Marcel/cairn/internal/csvfile"
	"github.com/Alexis-Marcel/cairn/internal/entry"
	"github.com/Alexis-Marcel/cairn/internal/fundarr"
	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

// ---- CSV import

// maxCSV bounds the file sent, inside its JSON.
const maxCSV = 24 << 20

// previewRows: how many rows the preview shows (all errors are counted).
const previewRows = 200

type importIn struct {
	CSV        string `json:"csv"`
	Account    string `json:"account,omitempty"`
	NewAccount string `json:"newAccount,omitempty"`
	FromFile   bool   `json:"fromFile,omitempty"` // each row's own account
}

type previewRow struct {
	Line      int           `json:"line"`
	Kind      string        `json:"kind,omitempty"`
	Time      *time.Time    `json:"time,omitempty"`
	Account   string        `json:"account,omitempty"`
	Base      *model.Amount `json:"base,omitempty"`
	Quote     *model.Amount `json:"quote,omitempty"`
	Error     string        `json:"error,omitempty"`
	Duplicate bool          `json:"duplicate,omitempty"`
}

// read parses the file and finds which rows are already stored.
func (s *Server) read(ctx context.Context, raw string) (*csvfile.File, map[string]bool, error) {
	f, err := csvfile.Read(strings.NewReader(raw))
	if err != nil {
		return nil, nil, &books.Error{Status: 422, Msg: err.Error()}
	}
	var ids []string
	for _, r := range f.Rows {
		if r.Error == "" {
			ids = append(ids, r.Activity.ID)
		}
	}
	have, err := s.St.Has(ctx, ids)
	return f, have, err
}

func (s *Server) importPreview(w http.ResponseWriter, r *http.Request) {
	var in importIn
	if !decode(w, r, maxCSV, &in) {
		return
	}
	f, have, err := s.read(r.Context(), in.CSV)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Transfers inside the file read as transfers.
	paired := map[string]bool{}
	for _, row := range f.Rows {
		if row.Error == "" && row.Activity.TransferWith != "" {
			paired[row.Activity.TransferWith] = true
		}
	}
	rows := []previewRow{}
	valid, errs, dups := 0, 0, 0
	names := map[string]bool{}
	for _, row := range f.Rows {
		p := previewRow{Line: row.Line, Account: row.Account, Error: row.Error}
		if row.Error != "" {
			errs++
		} else {
			a := row.Activity
			t := a.Time
			p.Time, p.Base, p.Quote = &t, &a.Base, a.Quote
			p.Kind = entry.Kind(a.Activity, paired[a.ID] && a.TransferWith != "")
			if have[a.ID] {
				p.Duplicate = true
				dups++
			} else {
				valid++
			}
			if row.Account != "" {
				names[row.Account] = true
			}
		}
		if len(rows) < previewRows || p.Error != "" && len(rows) < previewRows*2 {
			rows = append(rows, p)
		}
	}
	accounts := make([]string, 0, len(names))
	for n := range names {
		accounts = append(accounts, n)
	}
	sort.Strings(accounts)
	writeJSON(w, 200, map[string]any{
		"format": f.Format, "rows": rows,
		"total": len(f.Rows), "valid": valid, "errors": errs, "duplicates": dups,
		"fileAccounts": f.Accounts && len(accounts) > 0, "accounts": accounts,
	})
}

func (s *Server) importFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in importIn
	if !decode(w, r, maxCSV, &in) {
		return
	}
	f, _, err := s.read(ctx, in.CSV)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	account := ""
	switch {
	case in.FromFile:
		if !f.Accounts {
			writeErr(w, 400, "Des lignes ne disent pas leur compte : choisis-en un")
			return
		}
	default:
		if account, err = s.account(ctx, in.Account, in.NewAccount); err != nil {
			s.fail(w, r, err)
			return
		}
		if account == "" {
			writeErr(w, 400, "Choisis un compte")
			return
		}
	}
	var rows []store.Incoming
	errs := 0
	for _, row := range f.Rows {
		if row.Error != "" {
			errs++
			continue
		}
		rows = append(rows, store.Incoming{Activity: row.Activity, AccountName: row.Account})
	}
	added, skipped, err := s.St.Import(ctx, rows, account)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.Log.Info("import", "format", f.Format, "added", added, "skipped", skipped, "errors", errs)
	writeJSON(w, 200, map[string]int{"added": added, "skipped": skipped, "errors": errs})
}

func (s *Server) template(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="cairn-modele.csv"`)
	w.Write([]byte(csvfile.Template))
}

// ---- the link to Fundarr

func (s *Server) syncState(w http.ResponseWriter, r *http.Request) {
	v, err := s.syncView(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, v)
}

// syncFor runs a sync the page waits for, up to a while; it goes on even if
// the page goes away.
func (s *Server) syncFor(r *http.Request, d time.Duration) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), d)
	defer cancel()
	if err := s.Sync.Sync(ctx); err != nil {
		s.Log.Warn("fundarr sync", "err", err)
	}
}

// findFundarr looks for a Fundarr next to Cairn, to fill in its address.
func (s *Server) findFundarr(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"url": fundarr.Find(r.Context(), fundarr.Nearby)})
}

func (s *Server) link(w http.ResponseWriter, r *http.Request) {
	if s.FromEnv {
		writeErr(w, http.StatusConflict, "Réglé par FUNDARR_URL et FUNDARR_TOKEN")
		return
	}
	var in struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if !decode(w, r, 8<<10, &in) {
		return
	}
	u := fundarr.Normalize(in.URL)
	token := strings.TrimSpace(in.Token)
	if u == "" || !fundarr.ValidURL(u) {
		writeErr(w, 400, "Adresse invalide")
		return
	}
	if token == "" {
		writeErr(w, 400, "Jeton manquant")
		return
	}
	if err := s.Sync.Check(r.Context(), u, token); err != nil {
		var ce *fundarr.CheckError
		if errors.As(err, &ce) {
			writeErr(w, ce.Status, ce.Msg)
			return
		}
		writeErr(w, http.StatusBadGateway, "Adresse injoignable")
		return
	}
	if err := s.St.SetLink(r.Context(), u, token); err != nil {
		s.fail(w, r, err)
		return
	}
	s.Log.Info("linked", "fundarr", u)
	s.syncFor(r, 2*time.Minute)
	s.syncState(w, r)
}

func (s *Server) unlink(w http.ResponseWriter, r *http.Request) {
	if s.FromEnv {
		writeErr(w, http.StatusConflict, "Réglé par FUNDARR_URL et FUNDARR_TOKEN")
		return
	}
	var in struct {
		Keep bool `json:"keep"`
	}
	if !decode(w, r, 1<<10, &in) {
		return
	}
	if err := s.St.Unlink(r.Context(), in.Keep); err != nil {
		s.fail(w, r, err)
		return
	}
	s.Log.Info("unlinked", "kept", in.Keep)
	s.syncState(w, r)
}

func (s *Server) syncNow(w http.ResponseWriter, r *http.Request) {
	l, err := s.St.Link(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if l.URL == "" {
		writeErr(w, http.StatusConflict, "Pas relié à Fundarr")
		return
	}
	s.syncFor(r, 2*time.Minute)
	s.syncState(w, r)
}

func (s *Server) setSecret(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Secret string `json:"secret"`
	}
	if !decode(w, r, 2<<10, &in) {
		return
	}
	secret := strings.TrimSpace(in.Secret)
	if len(secret) > 200 || strings.ContainsAny(secret, " \t\r\n") {
		writeErr(w, 400, "Secret invalide")
		return
	}
	l, err := s.St.Link(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if l.URL == "" && secret != "" {
		writeErr(w, http.StatusConflict, "Relie d'abord Fundarr")
		return
	}
	if err := s.St.SetSecret(r.Context(), secret); err != nil {
		s.fail(w, r, err)
		return
	}
	s.syncState(w, r)
}
