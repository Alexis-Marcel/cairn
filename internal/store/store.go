// Package store keeps everything Cairn knows in one SQLite file: the
// accounts (comptes), the activities (entered by hand, imported from a CSV
// or copied from Fundarr), the link to Fundarr and the prices already asked.
//
// The file is readable by Cairn alone (0600): it holds the Fundarr token.
// Every write bumps Version, so what is computed from the activities knows
// when to start over.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Alexis-Marcel/fundarr/pkg/model"
	_ "modernc.org/sqlite" // pure Go: the image stays CGO-free
)

// File is the database, in the data folder.
const File = "cairn.db"

// Where an activity comes from.
const (
	Manual  = "manual"  // entered in Cairn
	Import  = "import"  // read from a CSV file
	Fundarr = "fundarr" // copied from Fundarr's changes feed: read-only here
)

// ErrNotFound: no such row.
var ErrNotFound = errors.New("not found")

// Store is the database. Safe for concurrent use.
type Store struct {
	db      *sql.DB
	version atomic.Int64
}

const schema = `
CREATE TABLE IF NOT EXISTS accounts (
	id        TEXT PRIMARY KEY,
	name      TEXT NOT NULL,
	origin    TEXT NOT NULL,             -- manual | fundarr
	source_id TEXT NOT NULL DEFAULT '',  -- Fundarr's source, for origin fundarr
	created   INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS accounts_source ON accounts(source_id) WHERE source_id != '';

CREATE TABLE IF NOT EXISTS activities (
	id            TEXT PRIMARY KEY,
	origin        TEXT NOT NULL,
	external      TEXT NOT NULL DEFAULT '', -- Fundarr's id, or the hash of an imported row
	account       TEXT NOT NULL REFERENCES accounts(id),
	time          INTEGER NOT NULL,         -- unix ms
	type          TEXT NOT NULL,
	asset         TEXT NOT NULL,            -- base symbol
	quote         TEXT NOT NULL DEFAULT '', -- quote symbol
	transfer_with TEXT NOT NULL DEFAULT '', -- the other side of a move between two accounts
	data          TEXT NOT NULL             -- the model.Activity, as JSON
);
CREATE INDEX IF NOT EXISTS activities_time ON activities(time DESC, id DESC);
CREATE INDEX IF NOT EXISTS activities_account ON activities(account, time);
CREATE INDEX IF NOT EXISTS activities_origin ON activities(origin);

CREATE TABLE IF NOT EXISTS settings (
	k TEXT PRIMARY KEY,
	v TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS prices (
	pair  TEXT NOT NULL,
	day   TEXT NOT NULL,
	close TEXT NOT NULL,
	PRIMARY KEY (pair, day)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS price_windows (
	pair    TEXT NOT NULL,
	"from"  TEXT NOT NULL,
	"to"    TEXT NOT NULL,
	fetched INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS price_windows_pair ON price_windows(pair);
`

// Open opens (or makes) the database in dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, File)
	// Made by hand first, so SQLite keeps these rights (and gives them to
	// the -wal and -shm files).
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	return open("file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_txlock=immediate")
}

// Memory opens a database that lives in memory, for tests.
func Memory() (*Store, error) {
	s, err := open("file::memory:?_pragma=foreign_keys(1)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	return s, nil
}

func open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if strings.Contains(dsn, ":memory:") {
		// Every connection would be a database of its own.
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(4)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	s := &Store{db: db}
	s.version.Store(time.Now().UnixNano())
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Version changes whenever an account or an activity does.
func (s *Store) Version() int64 { return s.version.Load() }

func (s *Store) changed() { s.version.Add(1) }

// Ping is for the health check.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// ---- accounts

// Account is a place coins are held: a platform, a wallet.
type Account struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Origin   string    `json:"origin"` // manual | fundarr
	SourceID string    `json:"-"`
	Created  time.Time `json:"-"`
	Count    int       `json:"count"` // activities
}

// Accounts lists every account, by name.
func (s *Store) Accounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.name, a.origin, a.source_id, a.created,
		(SELECT COUNT(*) FROM activities WHERE account = a.id) FROM accounts a ORDER BY a.name COLLATE NOCASE, a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		var a Account
		var created int64
		if err := rows.Scan(&a.ID, &a.Name, &a.Origin, &a.SourceID, &created, &a.Count); err != nil {
			return nil, err
		}
		a.Created = time.UnixMilli(created).UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}

// Account returns one account.
func (s *Store) Account(ctx context.Context, id string) (Account, error) {
	var a Account
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT id, name, origin, source_id, created,
		(SELECT COUNT(*) FROM activities WHERE account = accounts.id) FROM accounts WHERE id = ?`, id).
		Scan(&a.ID, &a.Name, &a.Origin, &a.SourceID, &created, &a.Count)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	a.Created = time.UnixMilli(created).UTC()
	return a, err
}

// cleanName trims an account name and bounds it.
func cleanName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	return name
}

// CreateAccount makes a manual account, or returns the one already named so
// (whatever the case).
func (s *Store) CreateAccount(ctx context.Context, name string) (Account, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback()
	a, err := accountNamed(ctx, tx, name)
	if err != nil {
		return a, err
	}
	if err := tx.Commit(); err != nil {
		return Account{}, err
	}
	s.changed()
	return a, nil
}

func accountNamed(ctx context.Context, tx *sql.Tx, name string) (Account, error) {
	name = cleanName(name)
	if name == "" {
		return Account{}, errors.New("account name missing")
	}
	var a Account
	err := tx.QueryRowContext(ctx, `SELECT id, name, origin FROM accounts WHERE name = ? COLLATE NOCASE ORDER BY origin = 'manual' DESC LIMIT 1`, name).
		Scan(&a.ID, &a.Name, &a.Origin)
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return a, err
	}
	a = Account{ID: "a-" + NewID()[:16], Name: name, Origin: Manual, Created: time.Now().UTC()}
	_, err = tx.ExecContext(ctx, `INSERT INTO accounts (id, name, origin, created) VALUES (?, ?, ?, ?)`,
		a.ID, a.Name, a.Origin, a.Created.UnixMilli())
	return a, err
}

// RenameAccount renames a manual account.
func (s *Store) RenameAccount(ctx context.Context, id, name string) error {
	name = cleanName(name)
	if name == "" {
		return errors.New("account name missing")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE accounts SET name = ? WHERE id = ? AND origin = 'manual'`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.changed()
	return nil
}

// ErrNotEmpty: an account still holds activities.
var ErrNotEmpty = errors.New("account not empty")

// DeleteAccount removes an empty manual account.
func (s *Store) DeleteAccount(ctx context.Context, id string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM activities WHERE account = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrNotEmpty
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ? AND origin = 'manual'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.changed()
	return nil
}

// ---- activities

// Activity is one stored activity. Activity.SourceID is the account's id:
// what pkg/model and pkg/portfolio group holdings by.
type Activity struct {
	model.Activity
	Origin       string `json:"origin"`
	External     string `json:"-"`
	TransferWith string `json:"transferWith,omitempty"`
}

const actCols = `id, origin, external, account, transfer_with, data`

func scanActivity(sc interface{ Scan(...any) error }) (Activity, error) {
	var a Activity
	var id, account, data string
	if err := sc.Scan(&id, &a.Origin, &a.External, &account, &a.TransferWith, &data); err != nil {
		return a, err
	}
	if err := json.Unmarshal([]byte(data), &a.Activity); err != nil {
		return a, fmt.Errorf("activity %s: %w", id, err)
	}
	a.ID, a.SourceID = id, account
	return a, nil
}

// All returns every activity, oldest first.
func (s *Store) All(ctx context.Context) ([]Activity, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+actCols+` FROM activities ORDER BY time, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Activity{}
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Get returns one activity.
func (s *Store) Get(ctx context.Context, id string) (Activity, error) {
	a, err := scanActivity(s.db.QueryRowContext(ctx, `SELECT `+actCols+` FROM activities WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// Has reports which of ids are stored.
func (s *Store) Has(ctx context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for start := 0; start < len(ids); start += 500 {
		chunk := ids[start:min(start+500, len(ids))]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		rows, err := s.db.QueryContext(ctx, `SELECT id FROM activities WHERE id IN (?`+strings.Repeat(",?", len(chunk)-1)+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Filter narrows List.
type Filter struct {
	Account string // account id
	Asset   string // symbol, base or quote
	Origin  string
	// Kind: what the base or quote is (crypto, security); with it, CashOf
	// are the accounts whose cash-only operations count too.
	Kind   string
	CashOf []string
	Query  string // words found in the type, an asset or its name, the account or the note
	Offset int
	Limit  int
}

// List returns a page of activities, newest first, and how many match.
func (s *Store) List(ctx context.Context, f Filter) ([]Activity, int, error) {
	where := []string{"1=1"}
	var args []any
	if f.Account != "" {
		where, args = append(where, "a.account = ?"), append(args, f.Account)
	}
	if f.Asset != "" {
		sym := strings.ToUpper(f.Asset)
		where, args = append(where, "(a.asset = ? OR a.quote = ?)"), append(args, sym, sym)
	}
	if f.Origin != "" {
		where, args = append(where, "a.origin = ?"), append(args, f.Origin)
	}
	if f.Kind != "" {
		cond := `(json_extract(a.data, '$.base.asset.kind') = ? OR json_extract(a.data, '$.quote.asset.kind') = ?`
		args = append(args, f.Kind, f.Kind)
		if len(f.CashOf) > 0 {
			cond += ` OR json_extract(a.data, '$.base.asset.kind') = 'fiat' AND COALESCE(json_extract(a.data, '$.quote.asset.kind'), 'fiat') = 'fiat'
				AND a.account IN (` + strings.TrimSuffix(strings.Repeat("?,", len(f.CashOf)), ",") + `)`
			for _, c := range f.CashOf {
				args = append(args, c)
			}
		}
		where = append(where, cond+")")
	}
	for _, w := range strings.Fields(f.Query) {
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(w) + "%"
		where = append(where, `(a.asset LIKE ? ESCAPE '\' OR a.quote LIKE ? ESCAPE '\' OR c.name LIKE ? ESCAPE '\'
			OR json_extract(a.data, '$.note') LIKE ? ESCAPE '\' OR a.type LIKE ? ESCAPE '\'
			OR json_extract(a.data, '$.base.asset.name') LIKE ? ESCAPE '\' OR json_extract(a.data, '$.quote.asset.name') LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like, like, like, like, like)
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM activities a JOIN accounts c ON c.id = a.account WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.origin, a.external, a.account, a.transfer_with, a.data
		FROM activities a JOIN accounts c ON c.id = a.account WHERE `+cond+` ORDER BY a.time DESC, a.id DESC LIMIT ? OFFSET ?`,
		append(args, limit, max(f.Offset, 0))...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Activity{}
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

// Assets lists the symbols used, base or quote, most used first.
func (s *Store) Assets(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sym FROM (SELECT asset AS sym FROM activities UNION ALL
		SELECT quote FROM activities WHERE quote != '') GROUP BY sym ORDER BY COUNT(*) DESC, sym`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var sym string
		if err := rows.Scan(&sym); err != nil {
			return nil, err
		}
		out = append(out, sym)
	}
	return out, rows.Err()
}

// LastOnDay is the time of the last activity of the UTC day d (zero if none).
func (s *Store) LastOnDay(ctx context.Context, d time.Time) (time.Time, error) {
	start := d.UTC().Truncate(24 * time.Hour)
	var ms sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(time) FROM activities WHERE time >= ? AND time < ?`,
		start.UnixMilli(), start.Add(24*time.Hour).UnixMilli()).Scan(&ms)
	if err != nil || !ms.Valid {
		return time.Time{}, err
	}
	return time.UnixMilli(ms.Int64).UTC(), nil
}

// Count is the number of activities.
func (s *Store) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM activities`).Scan(&n)
	return n, err
}

func put(ctx context.Context, tx *sql.Tx, a Activity) error {
	data, err := json.Marshal(stripped(a.Activity))
	if err != nil {
		return err
	}
	quote := ""
	if a.Quote != nil {
		quote = a.Quote.Asset.Symbol
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO activities (id, origin, external, account, time, type, asset, quote, transfer_with, data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET origin = excluded.origin, external = excluded.external, account = excluded.account,
			time = excluded.time, type = excluded.type, asset = excluded.asset, quote = excluded.quote,
			transfer_with = excluded.transfer_with, data = excluded.data`,
		a.ID, a.Origin, a.External, a.SourceID, a.Time.UnixMilli(), string(a.Type), a.Base.Asset.Symbol, quote, a.TransferWith, string(data))
	return err
}

// stripped is what is kept in data: the id and account have their columns.
func stripped(a model.Activity) model.Activity {
	a.ID, a.SourceID = "", ""
	a.Time = a.Time.UTC()
	return a
}

// Put adds or replaces activities, in one transaction. Each must name an
// existing account (SourceID) and have an id.
func (s *Store) Put(ctx context.Context, acts ...Activity) error {
	if len(acts) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, a := range acts {
		if a.ID == "" || a.SourceID == "" {
			return errors.New("activity without id or account")
		}
		if err := put(ctx, tx, a); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.changed()
	return nil
}

// Replace removes the activities old and adds acts, in one transaction:
// an edit that may turn one activity into two (a transfer) or back.
func (s *Store) Replace(ctx context.Context, old []string, acts ...Activity) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range old {
		if _, err := tx.ExecContext(ctx, `DELETE FROM activities WHERE id = ? AND origin != 'fundarr'`, id); err != nil {
			return err
		}
	}
	for _, a := range acts {
		if err := put(ctx, tx, a); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.changed()
	return nil
}

// Delete removes activities entered or imported in Cairn (never Fundarr's).
func (s *Store) Delete(ctx context.Context, ids ...string) error {
	return s.Replace(ctx, ids)
}

// NewID returns 24 random hex digits.
func NewID() string { return randomHex(12) }
