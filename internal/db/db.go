package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaFS embed.FS

type Store struct {
	DB *sqlx.DB
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_fk=1", path)
	db, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	st := &Store{DB: db}
	if err := st.migrate(); err != nil {
		return nil, err
	}
	return st, nil
}

func (s *Store) migrate() error {
	ddl, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(string(ddl))
	return err
}

func (s *Store) UpsertToken(token string) error {
	_, err := s.DB.Exec("INSERT INTO bot_credentials (id, token) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET token=excluded.token", token)
	return err
}

func (s *Store) GetToken() (string, error) {
	var token sql.NullString
	err := s.DB.Get(&token, "SELECT token FROM bot_credentials WHERE id=1")
	if err != nil {
		return "", err
	}
	if !token.Valid {
		return "", errors.New("no token in db")
	}
	return token.String, nil
}

func (s *Store) EnsureSettings(defaultTime string) error {
	_, err := s.DB.Exec("INSERT INTO settings (id, daily_time) VALUES (1, ?) ON CONFLICT(id) DO NOTHING", defaultTime)
	return err
}

func (s *Store) GetDailyTime() (string, error) {
	var t string
	err := s.DB.Get(&t, "SELECT daily_time FROM settings WHERE id=1")
	return t, err
}

func (s *Store) SetDailyTime(t string) error {
	_, err := s.DB.Exec("UPDATE settings SET daily_time=? WHERE id=1", t)
	return err
}

func (s *Store) UpsertChat(chatID int64, title string) error {
	_, err := s.DB.Exec("INSERT INTO chats (chat_id, title) VALUES (?, ?) ON CONFLICT(chat_id) DO UPDATE SET title=excluded.title", chatID, title)
	return err
}

func (s *Store) CreateOrGetTodaySession(chatID int64, date string, deadline time.Time) (int64, error) {
	// Idempotent creation pattern: try insert (ignored if exists), then ensure deadline populated/updated.
	_, err := s.DB.Exec("INSERT OR IGNORE INTO daily_sessions (chat_id, session_date, signup_deadline) VALUES (?, ?, ?)", chatID, date, deadline.UTC())
	if err != nil {
		return 0, fmt.Errorf("insert or ignore daily_session failed (chat=%d date=%s): %w", chatID, date, err)
	}
	// Update deadline if row existed without it or earlier smaller value (best-effort; ignore error).
	_, _ = s.DB.Exec("UPDATE daily_sessions SET signup_deadline=? WHERE chat_id=? AND session_date=? AND (signup_deadline IS NULL OR signup_deadline < ?)", deadline.UTC(), chatID, date, deadline.UTC())
	var id int64
	getErr := s.DB.Get(&id, "SELECT id FROM daily_sessions WHERE chat_id=? AND session_date=?", chatID, date)
	if getErr == nil {
		return id, nil
	}
	if errors.Is(getErr, sql.ErrNoRows) {
		// Unexpected: try explicit insert (may surface real constraint error)
		res, insErr := s.DB.Exec("INSERT INTO daily_sessions (chat_id, session_date, signup_deadline) VALUES (?, ?, ?)", chatID, date, deadline.UTC())
		if insErr == nil {
			id2, _ := res.LastInsertId()
			return id2, nil
		}
		// Gather diagnostics
		var cntSameDate int
		_ = s.DB.Get(&cntSameDate, "SELECT COUNT(1) FROM daily_sessions WHERE session_date=?", date)
		return 0, fmt.Errorf("session missing after insert-or-ignore retryFailed chat=%d date=%s rowsForDate=%d retryErr=%v", chatID, date, cntSameDate, insErr)
	}
	return 0, fmt.Errorf("select daily_session failed after insert-or-ignore (chat=%d date=%s): %w", chatID, date, getErr)
}

func (s *Store) SetInviteMessageID(sessionID int64, msgID int) error {
	_, err := s.DB.Exec("UPDATE daily_sessions SET invite_message_id=? WHERE id=?", msgID, sessionID)
	return err
}

// GetSessionByChatDate returns session id and invite_message_id if a session exists for given chat/date.
func (s *Store) GetSessionByChatDate(chatID int64, date string) (id int64, inviteMsgID sql.NullInt64, err error) {
	err = s.DB.QueryRowx("SELECT id, invite_message_id FROM daily_sessions WHERE chat_id=? AND session_date=?", chatID, date).Scan(&id, &inviteMsgID)
	return
}

func (s *Store) AddParticipant(sessionID int64, userID int64, username, display string) error {
	_, err := s.DB.Exec("INSERT INTO participants (session_id, user_id, username, display_name) VALUES (?, ?, ?, ?)", sessionID, userID, username, display)
	return err
}

func (s *Store) IsParticipant(sessionID int64, userID int64) (bool, error) {
	var cnt int
	err := s.DB.Get(&cnt, "SELECT COUNT(1) FROM participants WHERE session_id=? AND user_id=?", sessionID, userID)
	return cnt > 0, err
}

func (s *Store) GetOpenSessionsToClose(now time.Time) ([]int64, error) {
	rows, err := s.DB.Queryx("SELECT id FROM daily_sessions WHERE closed=0 AND signup_deadline <= ?", now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) GetSessionInfo(id int64) (chatID int64, date string, err error) {
	err = s.DB.QueryRowx("SELECT chat_id, session_date FROM daily_sessions WHERE id=?", id).Scan(&chatID, &date)
	return
}

func (s *Store) GetParticipants(sessionID int64) ([]Participant, error) {
	rows, err := s.DB.Queryx("SELECT user_id, COALESCE(username,''), COALESCE(display_name,'') FROM participants WHERE session_id=? ORDER BY id", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []Participant
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.UserID, &p.Username, &p.DisplayName); err != nil {
			return nil, err
		}
		res = append(res, p)
	}
	return res, rows.Err()
}

// HasAnySessionForDate returns true if there is at least one session for the given date (YYYY-MM-DD).
func (s *Store) HasAnySessionForDate(date string) (bool, error) {
	var x int
	err := s.DB.Get(&x, "SELECT 1 FROM daily_sessions WHERE session_date=? LIMIT 1", date)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type Participant struct {
	UserID      int64
	Username    string
	DisplayName string
}

func (s *Store) CloseSession(id int64) error {
	_, err := s.DB.Exec("UPDATE daily_sessions SET closed=1 WHERE id=?", id)
	return err
}

// CountSessionsByDate returns number of daily_sessions rows for a date.
func (s *Store) CountSessionsByDate(date string) (int, error) {
	var c int
	err := s.DB.Get(&c, "SELECT COUNT(1) FROM daily_sessions WHERE session_date=?", date)
	return c, err
}

// SessionOpen checks if session is not closed and deadline not passed at given time.
func (s *Store) SessionOpen(id int64, now time.Time) (bool, error) {
	var closed int
	var deadline time.Time
	err := s.DB.QueryRowx("SELECT closed, COALESCE(signup_deadline, CURRENT_TIMESTAMP) FROM daily_sessions WHERE id=?", id).Scan(&closed, &deadline)
	if err != nil {
		return false, err
	}
	if closed != 0 {
		return false, nil
	}
	if now.UTC().After(deadline.UTC()) {
		return false, nil
	}
	return true, nil
}

func (s *Store) WithTx(ctx context.Context, fn func(*sqlx.Tx) error) error {
	tx, err := s.DB.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
