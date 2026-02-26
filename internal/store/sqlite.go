package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/liangzd/hapi-lite/internal/session"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	return s, s.migrate()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			agent TEXT NOT NULL,
			directory TEXT NOT NULL,
			active INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			metadata_json TEXT,
			agent_state_json TEXT,
			permission_mode TEXT,
			model_mode TEXT
		);
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			seq INTEGER NOT NULL,
			role TEXT,
			content_json TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);
		CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, seq);
	`)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) CreateSession(req session.CreateSessionRequest) (*session.Session, error) {
	now := time.Now().UnixMilli()
	id := uuid.New().String()
	agent := req.Agent
	if agent == "" {
		agent = "claude"
	}

	meta := &session.Metadata{
		Path:   req.Directory,
		Host:   "localhost",
		Flavor: agent,
	}
	metaJSON, _ := json.Marshal(meta)

	var perm sql.NullString
	if req.Yolo {
		perm = sql.NullString{String: "yolo", Valid: true}
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, agent, directory, active, created_at, updated_at, metadata_json, model_mode, permission_mode) VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?)`,
		id, agent, req.Directory, now, now, string(metaJSON), req.Model, perm,
	)
	if err != nil {
		return nil, err
	}

	return &session.Session{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Active:    true,
		ActiveAt:  now,
		Metadata:  meta,
	}, nil
}

func (s *Store) GetSessions() ([]session.Session, error) {
	rows, err := s.db.Query(`SELECT id, agent, directory, active, created_at, updated_at, metadata_json, agent_state_json, permission_mode, model_mode FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []session.Session
	for rows.Next() {
		var (
			id, agent, dir       string
			active               int
			createdAt, updatedAt int64
			metaJSON, stateJSON  sql.NullString
			permMode, modelMode  sql.NullString
		)
		if err := rows.Scan(&id, &agent, &dir, &active, &createdAt, &updatedAt, &metaJSON, &stateJSON, &permMode, &modelMode); err != nil {
			return nil, err
		}

		s := session.Session{
			ID: id, CreatedAt: createdAt, UpdatedAt: updatedAt,
			Active: active == 1, ActiveAt: createdAt,
		}
		if metaJSON.Valid {
			var m session.Metadata
			json.Unmarshal([]byte(metaJSON.String), &m)
			s.Metadata = &m
		}
		if stateJSON.Valid {
			var a session.AgentState
			json.Unmarshal([]byte(stateJSON.String), &a)
			s.AgentState = &a
		}
		if permMode.Valid {
			s.PermissionMode = permMode.String
		}
		if modelMode.Valid {
			s.ModelMode = modelMode.String
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (s *Store) GetSession(id string) (*session.Session, error) {
	var (
		agent, dir           string
		active               int
		createdAt, updatedAt int64
		metaJSON, stateJSON  sql.NullString
		permMode, modelMode  sql.NullString
	)
	err := s.db.QueryRow(
		`SELECT agent, directory, active, created_at, updated_at, metadata_json, agent_state_json, permission_mode, model_mode FROM sessions WHERE id = ?`, id,
	).Scan(&agent, &dir, &active, &createdAt, &updatedAt, &metaJSON, &stateJSON, &permMode, &modelMode)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	sess := &session.Session{
		ID: id, CreatedAt: createdAt, UpdatedAt: updatedAt,
		Active: active == 1, ActiveAt: createdAt,
	}
	if metaJSON.Valid {
		var m session.Metadata
		json.Unmarshal([]byte(metaJSON.String), &m)
		sess.Metadata = &m
	}
	if stateJSON.Valid {
		var a session.AgentState
		json.Unmarshal([]byte(stateJSON.String), &a)
		sess.AgentState = &a
	}
	if permMode.Valid {
		sess.PermissionMode = permMode.String
	}
	if modelMode.Valid {
		sess.ModelMode = modelMode.String
	}
	return sess, nil
}

func (s *Store) DeleteSession(id string) error {
	tx, _ := s.db.Begin()
	tx.Exec(`DELETE FROM messages WHERE session_id = ?`, id)
	tx.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return tx.Commit()
}

func (s *Store) UpdateSessionMeta(id string, meta *session.Metadata) error {
	metaJSON, _ := json.Marshal(meta)
	_, err := s.db.Exec(`UPDATE sessions SET metadata_json = ?, updated_at = ? WHERE id = ?`,
		string(metaJSON), time.Now().UnixMilli(), id)
	return err
}

func (s *Store) SetSessionActive(id string, active bool) error {
	v := 0
	if active {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE sessions SET active = ?, updated_at = ? WHERE id = ?`, v, time.Now().UnixMilli(), id)
	return err
}

func (s *Store) SetSessionPermissionMode(id string, mode string) error {
	_, err := s.db.Exec(`UPDATE sessions SET permission_mode = ?, updated_at = ? WHERE id = ?`,
		mode, time.Now().UnixMilli(), id)
	return err
}

func (s *Store) SetSessionModelMode(id string, model string) error {
	_, err := s.db.Exec(`UPDATE sessions SET model_mode = ?, updated_at = ? WHERE id = ?`,
		model, time.Now().UnixMilli(), id)
	return err
}

func (s *Store) RenameSession(id string, name string) error {
	sess, err := s.GetSession(id)
	if err != nil {
		return err
	}
	if sess == nil {
		return sql.ErrNoRows
	}
	meta := sess.Metadata
	if meta == nil {
		meta = &session.Metadata{}
	}
	meta.Name = name
	return s.UpdateSessionMeta(id, meta)
}

func (s *Store) InsertMessage(msg session.Message) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO messages (id, session_id, seq, content_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, msg.Seq, string(msg.Content), msg.CreatedAt,
	)
	return err
}

func (s *Store) GetMessages(sessionID string, limit int, beforeSeq *int64) ([]session.Message, error) {
	var rows *sql.Rows
	var err error
	if beforeSeq != nil {
		rows, err = s.db.Query(
			`SELECT id, session_id, seq, content_json, created_at FROM messages WHERE session_id = ? AND seq < ? ORDER BY seq DESC LIMIT ?`,
			sessionID, *beforeSeq, limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, session_id, seq, content_json, created_at FROM messages WHERE session_id = ? ORDER BY seq DESC LIMIT ?`,
			sessionID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []session.Message
	for rows.Next() {
		var m session.Message
		var content string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Seq, &content, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Content = json.RawMessage(content)
		msgs = append(msgs, m)
	}
	// reverse to ascending order
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (s *Store) GetMessageCount(sessionID string) (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sessionID).Scan(&count)
	return count, err
}

func (s *Store) GetMessageCountBefore(sessionID string, beforeSeq int64) (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ? AND seq < ?`, sessionID, beforeSeq).Scan(&count)
	return count, err
}

func (s *Store) ReindexMessageSeqs() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		WITH ranked AS (
			SELECT
				id,
				ROW_NUMBER() OVER (
					PARTITION BY session_id
					ORDER BY created_at ASC, id ASC
				) AS new_seq
			FROM messages
		)
		UPDATE messages
		SET seq = (
			SELECT ranked.new_seq
			FROM ranked
			WHERE ranked.id = messages.id
		)
	`)
	if err != nil {
		return err
	}

	return tx.Commit()
}
