package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Alexis-Marcel/fundarr/pkg/api"
	"github.com/Alexis-Marcel/fundarr/pkg/price"
)

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- import

// Incoming is an activity read from a file, before it has an account.
type Incoming struct {
	Activity
	AccountName string
}

// Import adds activities to account (or, when account is "", to the account
// named by each, made when missing), skipping those already stored. All or
// nothing.
func (s *Store) Import(ctx context.Context, rows []Incoming, account string) (added, skipped int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if account != "" {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE id = ?`, account).Scan(&n); err != nil {
			return 0, 0, err
		}
		if n == 0 {
			return 0, 0, ErrNotFound
		}
	}
	named := map[string]string{}
	for _, r := range rows {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM activities WHERE id = ?`, r.ID).Scan(&n); err != nil {
			return 0, 0, err
		}
		if n > 0 {
			skipped++
			continue
		}
		a := r.Activity
		a.SourceID = account
		if a.SourceID == "" {
			key := strings.ToLower(cleanName(r.AccountName))
			if named[key] == "" {
				acc, err := accountNamed(ctx, tx, r.AccountName)
				if err != nil {
					return 0, 0, err
				}
				named[key] = acc.ID
			}
			a.SourceID = named[key]
		}
		if err := put(ctx, tx, a); err != nil {
			return 0, 0, err
		}
		added++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	if added > 0 {
		s.changed()
	}
	return added, skipped, nil
}

// ---- settings

func (s *Store) setting(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, k string) (string, error) {
	var v string
	err := q.QueryRowContext(ctx, `SELECT v FROM settings WHERE k = ?`, k).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func setSettings(ctx context.Context, tx *sql.Tx, kv map[string]string) error {
	for k, v := range kv {
		var err error
		if v == "" {
			_, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE k = ?`, k)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO settings (k, v) VALUES (?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`, k, v)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Set writes settings ("" removes one), in one transaction.
func (s *Store) Set(ctx context.Context, kv map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setSettings(ctx, tx, kv); err != nil {
		return err
	}
	return tx.Commit()
}

// Setting reads one setting ("" when unset).
func (s *Store) Setting(ctx context.Context, k string) (string, error) {
	return s.setting(ctx, s.db, k)
}

// Link is the Fundarr to copy from, and where the copy stands.
type Link struct {
	URL, Token string
	Cursor     string
	Secret     string // signs Fundarr's notifications; "" turns them off
	LastSync   time.Time
	LastError  string
}

// Setting keys of the link.
const (
	kURL      = "fundarr.url"
	kToken    = "fundarr.token"
	kCursor   = "fundarr.cursor"
	kSecret   = "fundarr.secret"
	kLastSync = "fundarr.last_sync"
	kLastErr  = "fundarr.last_error"
)

// Link reads the link to Fundarr (URL "" when there is none).
func (s *Store) Link(ctx context.Context) (Link, error) {
	var l Link
	rows, err := s.db.QueryContext(ctx, `SELECT k, v FROM settings WHERE k LIKE 'fundarr.%'`)
	if err != nil {
		return l, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return l, err
		}
		switch k {
		case kURL:
			l.URL = v
		case kToken:
			l.Token = v
		case kCursor:
			l.Cursor = v
		case kSecret:
			l.Secret = v
		case kLastSync:
			if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
				l.LastSync = time.UnixMilli(ms).UTC()
			}
		case kLastErr:
			l.LastError = v
		}
	}
	return l, rows.Err()
}

// SetLink links to a Fundarr. Another address is another history: what was
// copied from the last one goes, and the copy starts over.
func (s *Store) SetLink(ctx context.Context, url, token string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old, err := s.setting(ctx, tx, kURL)
	if err != nil {
		return err
	}
	kv := map[string]string{kURL: url, kToken: token}
	dropped := false
	if old != url {
		if err := dropFundarr(ctx, tx); err != nil {
			return err
		}
		kv[kCursor], kv[kLastSync], kv[kLastErr] = "", "", ""
		dropped = true
	}
	if err := setSettings(ctx, tx, kv); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if dropped {
		s.changed()
	}
	return nil
}

// SetSecret keeps the notification secret ("" turns notifications off).
func (s *Store) SetSecret(ctx context.Context, secret string) error {
	return s.Set(ctx, map[string]string{kSecret: secret})
}

// Unlink forgets the Fundarr. keep: what was copied stays, as imported
// activities in accounts of their own that can be edited; otherwise it goes.
func (s *Store) Unlink(ctx context.Context, keep bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if keep {
		if _, err := tx.ExecContext(ctx, `UPDATE activities SET origin = 'import' WHERE origin = 'fundarr'`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET origin = 'manual', source_id = '' WHERE origin = 'fundarr'`); err != nil {
			return err
		}
	} else if err := dropFundarr(ctx, tx); err != nil {
		return err
	}
	if err := setSettings(ctx, tx, map[string]string{kURL: "", kToken: "", kCursor: "", kSecret: "", kLastSync: "", kLastErr: ""}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.changed()
	return nil
}

func dropFundarr(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM activities WHERE origin = 'fundarr'`); err != nil {
		return err
	}
	return dropEmptyFundarrAccounts(ctx, tx)
}

func dropEmptyFundarrAccounts(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM accounts WHERE origin = 'fundarr'
		AND NOT EXISTS (SELECT 1 FROM activities WHERE account = accounts.id)`)
	return err
}

// DropFundarr removes what was copied from Fundarr and its cursor: the copy
// starts over (Fundarr answered 410).
func (s *Store) DropFundarr(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := dropFundarr(ctx, tx); err != nil {
		return err
	}
	if err := setSettings(ctx, tx, map[string]string{kCursor: ""}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.changed()
	return nil
}

// FundarrAccountID is the account of a Fundarr source.
func FundarrAccountID(sourceID string) string { return "f-" + sourceID }

// split: the kinds of Fundarr source whose sub-accounts are accounts of
// their own (a broker's securities account and its PEA), with the name of
// the main one. A wallet's chains stay one account.
var split = map[string]string{"traderepublic": "CTO"}

// fundarrAccount is the account an activity of Fundarr goes to: its
// source, or the source's sub-account when they are told apart.
func fundarrAccount(a api.Activity) (id, sourceID, name string) {
	src := a.Source.ID
	if src == "" {
		src = a.SourceID
	}
	name = cleanName(a.Source.Name)
	if name == "" {
		name = src
	}
	main, ok := split[a.Source.Kind]
	if !ok || src == "" {
		return FundarrAccountID(src), src, name
	}
	if a.Account == "" {
		return FundarrAccountID(src), src, cleanName(name + " " + main)
	}
	sub := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, strings.ToLower(a.Account))
	return FundarrAccountID(src) + "." + sub, src + "/" + a.Account, cleanName(name + " " + a.Account)
}

// ApplyChanges applies a page of Fundarr's changes feed and keeps its
// cursor, in one transaction: a crash leaves the copy as it was before the
// page or after it.
func (s *Store) ApplyChanges(ctx context.Context, page api.Changes) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	seen := map[string]bool{}
	for _, ch := range page.Changes {
		switch {
		case ch.Op == api.Remove:
			if _, err := tx.ExecContext(ctx, `DELETE FROM activities WHERE id = ? AND origin = 'fundarr'`, ch.ID); err != nil {
				return err
			}
		case ch.Op == api.Upsert && ch.Activity != nil:
			a := *ch.Activity
			acc, src, name := fundarrAccount(a)
			if src == "" {
				continue
			}
			if !seen[acc] {
				if _, err := tx.ExecContext(ctx, `INSERT INTO accounts (id, name, origin, source_id, created) VALUES (?, ?, 'fundarr', ?, ?)
					ON CONFLICT(id) DO UPDATE SET name = excluded.name, origin = 'fundarr', source_id = excluded.source_id`,
					acc, name, src, time.Now().UnixMilli()); err != nil {
					return err
				}
				seen[acc] = true
			}
			m := a.Activity
			m.ID = ch.ID
			m.SourceID = acc
			if err := put(ctx, tx, Activity{Activity: m, Origin: Fundarr, External: ch.ID, TransferWith: a.TransferWith}); err != nil {
				return err
			}
		}
	}
	if err := dropEmptyFundarrAccounts(ctx, tx); err != nil {
		return err
	}
	if err := setSettings(ctx, tx, map[string]string{kCursor: page.Cursor}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if len(page.Changes) > 0 {
		s.changed()
	}
	return nil
}

// SyncDone records how the last copy went.
func (s *Store) SyncDone(ctx context.Context, at time.Time, err error) error {
	kv := map[string]string{kLastErr: ""}
	if err != nil {
		kv[kLastErr] = err.Error()
	} else {
		kv[kLastSync] = strconv.FormatInt(at.UnixMilli(), 10)
	}
	return s.Set(ctx, kv)
}

// ---- the link of earlier versions

// LegacyFile is where Cairn kept the link to Fundarr before it had a database.
const LegacyFile = "cairn.json"

// MigrateLegacy moves the link of cairn.json in dir into the database, once,
// and removes the file (it holds the token).
func (s *Store) MigrateLegacy(ctx context.Context, dir string) (bool, error) {
	path := filepath.Join(dir, LegacyFile)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var c struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return false, err
	}
	l, err := s.Link(ctx)
	if err != nil {
		return false, err
	}
	if l.URL == "" && c.URL != "" {
		if err := s.SetLink(ctx, c.URL, c.Token); err != nil {
			return false, err
		}
	}
	return true, os.Remove(path)
}

// ---- prices (price.Cache)

var _ price.Cache = (*Store)(nil)

// PriceSeries returns the closes kept for a pair.
func (s *Store) PriceSeries(ctx context.Context, pair string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT day, close FROM prices WHERE pair = ?`, pair)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var d, c string
		if err := rows.Scan(&d, &c); err != nil {
			return nil, err
		}
		out[d] = c
	}
	return out, rows.Err()
}

// PriceWindows returns the ranges already asked for a pair.
func (s *Store) PriceWindows(ctx context.Context, pair string) ([]price.Window, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT "from", "to", fetched FROM price_windows WHERE pair = ?`, pair)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []price.Window
	for rows.Next() {
		var w price.Window
		var ms int64
		if err := rows.Scan(&w.From, &w.To, &ms); err != nil {
			return nil, err
		}
		w.Fetched = time.UnixMilli(ms).UTC()
		out = append(out, w)
	}
	return out, rows.Err()
}

// PutPrices keeps closes and the window they were asked for.
func (s *Store) PutPrices(ctx context.Context, pair string, w price.Window, closes map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for d, c := range closes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO prices (pair, day, close) VALUES (?, ?, ?)
			ON CONFLICT(pair, day) DO UPDATE SET close = excluded.close`, pair, d, c); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO price_windows (pair, "from", "to", fetched) VALUES (?, ?, ?, ?)`,
		pair, w.From, w.To, w.Fetched.UnixMilli()); err != nil {
		return err
	}
	// The windows of a pair only grow; a few dozen are plenty.
	if _, err := tx.ExecContext(ctx, `DELETE FROM price_windows WHERE pair = ? AND rowid NOT IN
		(SELECT rowid FROM price_windows WHERE pair = ? ORDER BY fetched DESC LIMIT 50)`, pair, pair); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- migrations

// kSplit: Fundarr's copy was made again since a broker's sub-accounts (CTO,
// PEA) are accounts of their own.
const kSplit = "migrated.split_accounts"

// migrate brings an earlier database up to date, once.
func (s *Store) migrate(ctx context.Context) error {
	done, err := s.setting(ctx, s.db, kSplit)
	if err != nil || done != "" {
		return err
	}
	// Copied before: read the whole history again, so every activity lands
	// in its sub-account (put moves it) and accounts get their names.
	return s.Set(ctx, map[string]string{kSplit: "1", kCursor: ""})
}
