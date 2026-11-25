package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yeegz/repropack/format"
	_ "modernc.org/sqlite"
)

// Store keeps drafts (reports being captured) in a local SQLite database.
// Events are written as they happen, so a crash or quit never loses a
// capture in progress. Nothing here leaves the machine.
type Store struct {
	db  *sql.DB
	dir string // root data directory
}

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS drafts(
  id TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  steps TEXT NOT NULL DEFAULT '[]',
  expected TEXT NOT NULL DEFAULT '',
  actual TEXT NOT NULL DEFAULT '',
  frequency TEXT NOT NULL DEFAULT '',
  target_pid INTEGER NOT NULL DEFAULT 0,
  target_name TEXT NOT NULL DEFAULT '',
  manual_redactions INTEGER NOT NULL DEFAULT 0,
  exported_path TEXT NOT NULL DEFAULT '',
  exported_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS events(
  draft_id TEXT NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL,
  ts INTEGER NOT NULL,
  body TEXT NOT NULL,
  PRIMARY KEY(draft_id, seq)
);
CREATE TABLE IF NOT EXISTS attachments(
  draft_id TEXT NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
  id TEXT NOT NULL,
  file TEXT NOT NULL,
  media_type TEXT NOT NULL,
  role TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  width INTEGER NOT NULL DEFAULT 0,
  height INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  annotated INTEGER NOT NULL DEFAULT 0,
  reviewed INTEGER NOT NULL DEFAULT 0,
  note TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(draft_id, id)
);
CREATE TABLE IF NOT EXISTS notes(
  draft_id TEXT NOT NULL REFERENCES drafts(id) ON DELETE CASCADE,
  id TEXT NOT NULL,
  at INTEGER NOT NULL,
  text TEXT NOT NULL,
  PRIMARY KEY(draft_id, id)
);
CREATE TABLE IF NOT EXISTS recent_reports(
  path TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  opened_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS settings(
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

// OpenStore opens (or creates) the store under dir with 0700 permissions.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "drafts"), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "repropack.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // serialise writers; the app's write rate is tiny
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: %w", err)
	}
	os.Chmod(filepath.Join(dir, "repropack.db"), 0o600)
	return &Store{db: db, dir: dir}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DraftDir is where a draft's attachment files live.
func (s *Store) DraftDir(id string) string { return filepath.Join(s.dir, "drafts", id) }

// Draft is the editable description of a report in progress.
type Draft struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Steps       []string  `json:"steps"`
	Expected    string    `json:"expected"`
	Actual      string    `json:"actual"`
	Frequency   string    `json:"frequency"`
	TargetPID   int       `json:"targetPid"`
	TargetName  string    `json:"targetName"`
	Manual      int       `json:"manualRedactions"`
	Exported    string    `json:"exportedPath"`
	ExportedAt  time.Time `json:"exportedAt"`
	EventCount  int       `json:"eventCount"`
	AttachCount int       `json:"attachmentCount"`
}

func (s *Store) CreateDraft() (*Draft, error) {
	id := format.NewID()
	now := time.Now()
	if err := os.MkdirAll(s.DraftDir(id), 0o700); err != nil {
		return nil, err
	}
	_, err := s.db.Exec(`INSERT INTO drafts(id, created_at, updated_at) VALUES(?,?,?)`, id, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return nil, err
	}
	return s.GetDraft(id)
}

var errNotFound = errors.New("not found")

func (s *Store) GetDraft(id string) (*Draft, error) {
	row := s.db.QueryRow(`SELECT id, created_at, updated_at, title, description, steps, expected, actual, frequency, target_pid, target_name, manual_redactions, exported_path, exported_at,
		(SELECT count(*) FROM events e WHERE e.draft_id = d.id), (SELECT count(*) FROM attachments a WHERE a.draft_id = d.id) FROM drafts d WHERE id = ?`, id)
	return scanDraft(row)
}

type scanner interface{ Scan(...any) error }

func scanDraft(row scanner) (*Draft, error) {
	var d Draft
	var c, u, ex int64
	var steps string
	err := row.Scan(&d.ID, &c, &u, &d.Title, &d.Description, &steps, &d.Expected, &d.Actual, &d.Frequency, &d.TargetPID, &d.TargetName, &d.Manual, &d.Exported, &ex, &d.EventCount, &d.AttachCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	d.CreatedAt, d.UpdatedAt = time.UnixMilli(c), time.UnixMilli(u)
	if ex > 0 {
		d.ExportedAt = time.UnixMilli(ex)
	}
	json.Unmarshal([]byte(steps), &d.Steps)
	if d.Steps == nil {
		d.Steps = []string{}
	}
	return &d, nil
}

func (s *Store) ListDrafts() ([]Draft, error) {
	rows, err := s.db.Query(`SELECT id, created_at, updated_at, title, description, steps, expected, actual, frequency, target_pid, target_name, manual_redactions, exported_path, exported_at,
		(SELECT count(*) FROM events e WHERE e.draft_id = d.id), (SELECT count(*) FROM attachments a WHERE a.draft_id = d.id) FROM drafts d ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Draft{}
	for rows.Next() {
		d, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// DraftText is the user-editable subset of a draft.
type DraftText struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	Expected    string   `json:"expected"`
	Actual      string   `json:"actual"`
	Frequency   string   `json:"frequency"`
}

func (s *Store) UpdateDraft(id string, t DraftText) error {
	clip := func(v string, n int) string {
		if len(v) > n {
			return v[:n]
		}
		return v
	}
	if len(t.Steps) > 50 {
		t.Steps = t.Steps[:50]
	}
	for i := range t.Steps {
		t.Steps[i] = clip(t.Steps[i], 2000)
	}
	steps, _ := json.Marshal(t.Steps)
	res, err := s.db.Exec(`UPDATE drafts SET title=?, description=?, steps=?, expected=?, actual=?, frequency=?, updated_at=? WHERE id=?`,
		clip(t.Title, 300), clip(t.Description, 20000), string(steps), clip(t.Expected, 5000), clip(t.Actual, 5000), clip(t.Frequency, 40), time.Now().UnixMilli(), id)
	return mustAffect(res, err)
}

func mustAffect(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound
	}
	return nil
}

func (s *Store) SetTarget(id string, pid int, name string) error {
	res, err := s.db.Exec(`UPDATE drafts SET target_pid=?, target_name=?, updated_at=? WHERE id=?`, pid, name, time.Now().UnixMilli(), id)
	return mustAffect(res, err)
}

func (s *Store) MarkExported(id, path string) error {
	_, err := s.db.Exec(`UPDATE drafts SET exported_path=?, exported_at=?, updated_at=? WHERE id=?`, path, time.Now().UnixMilli(), time.Now().UnixMilli(), id)
	return err
}

func (s *Store) AddManual(id string, n int) error {
	_, err := s.db.Exec(`UPDATE drafts SET manual_redactions = manual_redactions + ? WHERE id=?`, n, id)
	return err
}

// DeleteDraft removes a draft and its files.
func (s *Store) DeleteDraft(id string) error {
	if !validDraftID(id) {
		return errNotFound
	}
	if _, err := s.db.Exec(`DELETE FROM drafts WHERE id=?`, id); err != nil {
		return err
	}
	return os.RemoveAll(s.DraftDir(id))
}

func validDraftID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r == '-') {
			return false
		}
	}
	return true
}

// AddEvent appends an event with the next sequence number.
func (s *Store) AddEvent(draftID string, e format.Event) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var next uint64
	if err := tx.QueryRow(`SELECT coalesce(max(seq),0)+1 FROM events WHERE draft_id=?`, draftID).Scan(&next); err != nil {
		return err
	}
	e.Seq = next
	b, _ := json.Marshal(e)
	if _, err := tx.Exec(`INSERT INTO events(draft_id, seq, ts, body) VALUES(?,?,?,?)`, draftID, e.Seq, e.Time.UnixNano(), string(b)); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE drafts SET updated_at=? WHERE id=?`, time.Now().UnixMilli(), draftID); err != nil {
		return err
	}
	return tx.Commit()
}

// AddEvents inserts many events in one transaction (e.g. a log file).
func (s *Store) AddEvents(draftID string, es []format.Event) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var next uint64
	if err := tx.QueryRow(`SELECT coalesce(max(seq),0)+1 FROM events WHERE draft_id=?`, draftID).Scan(&next); err != nil {
		return err
	}
	st, err := tx.Prepare(`INSERT INTO events(draft_id, seq, ts, body) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, e := range es {
		e.Seq = next
		next++
		b, _ := json.Marshal(e)
		if _, err := st.Exec(draftID, e.Seq, e.Time.UnixNano(), string(b)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Events returns a draft's events ordered by time, then seq.
func (s *Store) Events(draftID string) ([]format.Event, error) {
	rows, err := s.db.Query(`SELECT body FROM events WHERE draft_id=? ORDER BY ts, seq`, draftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []format.Event
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var e format.Event
		if json.Unmarshal([]byte(b), &e) == nil {
			out = append(out, e)
		}
	}
	return out, rows.Err()
}

// DraftAttachment is one captured file.
type DraftAttachment struct {
	ID         string    `json:"id"`
	File       string    `json:"-"`
	MediaType  string    `json:"mediaType"`
	Role       string    `json:"role"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"createdAt"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	DurationMS int64     `json:"durationMs"`
	Annotated  bool      `json:"annotated"`
	Reviewed   bool      `json:"reviewed"`
	Note       string    `json:"note"`
	Size       int64     `json:"size"`
	URL        string    `json:"url"`
}

func (s *Store) AddAttachment(draftID string, a DraftAttachment) error {
	_, err := s.db.Exec(`INSERT INTO attachments(draft_id, id, file, media_type, role, title, created_at, width, height, duration_ms) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		draftID, a.ID, a.File, a.MediaType, a.Role, a.Title, a.CreatedAt.UnixMilli(), a.Width, a.Height, a.DurationMS)
	return err
}

func (s *Store) Attachments(draftID string) ([]DraftAttachment, error) {
	rows, err := s.db.Query(`SELECT id, file, media_type, role, title, created_at, width, height, duration_ms, annotated, reviewed, note FROM attachments WHERE draft_id=? ORDER BY created_at`, draftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DraftAttachment{}
	for rows.Next() {
		var a DraftAttachment
		var c int64
		if err := rows.Scan(&a.ID, &a.File, &a.MediaType, &a.Role, &a.Title, &c, &a.Width, &a.Height, &a.DurationMS, &a.Annotated, &a.Reviewed, &a.Note); err != nil {
			return nil, err
		}
		a.CreatedAt = time.UnixMilli(c)
		if st, err := os.Stat(filepath.Join(s.DraftDir(draftID), a.File)); err == nil {
			a.Size = st.Size()
		}
		a.URL = "/media/draft/" + draftID + "/" + a.ID
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Attachment(draftID, id string) (*DraftAttachment, error) {
	as, err := s.Attachments(draftID)
	if err != nil {
		return nil, err
	}
	for i := range as {
		if as[i].ID == id {
			return &as[i], nil
		}
	}
	return nil, errNotFound
}

func (s *Store) UpdateAttachment(draftID, id string, set map[string]any) error {
	allowed := map[string]bool{"annotated": true, "reviewed": true, "note": true, "title": true, "file": true, "width": true, "height": true, "media_type": true}
	for k, v := range set {
		if !allowed[k] {
			return fmt.Errorf("field %q not editable", k)
		}
		res, err := s.db.Exec(`UPDATE attachments SET `+k+`=? WHERE draft_id=? AND id=?`, v, draftID, id)
		if err := mustAffect(res, err); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteAttachment(draftID, id string) error {
	a, err := s.Attachment(draftID, id)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM attachments WHERE draft_id=? AND id=?`, draftID, id); err != nil {
		return err
	}
	return os.Remove(filepath.Join(s.DraftDir(draftID), a.File))
}

// Note is a timestamped user note.
type Note struct {
	ID   string    `json:"id"`
	At   time.Time `json:"at"`
	Text string    `json:"text"`
}

func (s *Store) AddNote(draftID, text string) (*Note, error) {
	if len(text) > 5000 {
		text = text[:5000]
	}
	n := Note{ID: format.ShortID(), At: time.Now(), Text: text}
	_, err := s.db.Exec(`INSERT INTO notes(draft_id, id, at, text) VALUES(?,?,?,?)`, draftID, n.ID, n.At.UnixMilli(), n.Text)
	return &n, err
}

func (s *Store) Notes(draftID string) ([]Note, error) {
	rows, err := s.db.Query(`SELECT id, at, text FROM notes WHERE draft_id=? ORDER BY at`, draftID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		var at int64
		if err := rows.Scan(&n.ID, &at, &n.Text); err != nil {
			return nil, err
		}
		n.At = time.UnixMilli(at)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) DeleteNote(draftID, id string) error {
	res, err := s.db.Exec(`DELETE FROM notes WHERE draft_id=? AND id=?`, draftID, id)
	return mustAffect(res, err)
}

// Recent reports opened in the viewer.
type Recent struct {
	Path     string    `json:"path"`
	Title    string    `json:"title"`
	OpenedAt time.Time `json:"openedAt"`
	Exists   bool      `json:"exists"`
}

func (s *Store) TouchRecent(path, title string) error {
	_, err := s.db.Exec(`INSERT INTO recent_reports(path, title, opened_at) VALUES(?,?,?) ON CONFLICT(path) DO UPDATE SET title=excluded.title, opened_at=excluded.opened_at`, path, title, time.Now().UnixMilli())
	s.db.Exec(`DELETE FROM recent_reports WHERE path NOT IN (SELECT path FROM recent_reports ORDER BY opened_at DESC LIMIT 20)`)
	return err
}

func (s *Store) Recents() ([]Recent, error) {
	rows, err := s.db.Query(`SELECT path, title, opened_at FROM recent_reports ORDER BY opened_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recent{}
	for rows.Next() {
		var r Recent
		var at int64
		if err := rows.Scan(&r.Path, &r.Title, &at); err != nil {
			return nil, err
		}
		r.OpenedAt = time.UnixMilli(at)
		_, err := os.Stat(r.Path)
		r.Exists = err == nil
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ForgetRecent(path string) error {
	_, err := s.db.Exec(`DELETE FROM recent_reports WHERE path=?`, path)
	return err
}

func (s *Store) Setting(key, def string) string {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key, value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// Cleanup deletes drafts not touched for longer than retention. It returns
// how many were removed. retention <= 0 disables cleanup.
func (s *Store) Cleanup(retention time.Duration) (int, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-retention).UnixMilli()
	rows, err := s.db.Query(`SELECT id FROM drafts WHERE updated_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := s.DeleteDraft(id); err != nil {
			return 0, err
		}
	}
	// Remove orphan directories (e.g. from a crash between steps).
	ents, _ := os.ReadDir(filepath.Join(s.dir, "drafts"))
	for _, e := range ents {
		if _, err := s.GetDraft(e.Name()); errors.Is(err, errNotFound) && validDraftID(e.Name()) {
			os.RemoveAll(filepath.Join(s.dir, "drafts", e.Name()))
		}
	}
	return len(ids), nil
}
