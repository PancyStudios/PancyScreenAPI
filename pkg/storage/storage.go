package storage

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no CGO required
)

var db *sql.DB

// HistoryEntry represents a single screenshot event stored in history.
type HistoryEntry struct {
	ID        int64  `json:"id"`
	UserID    string `json:"user_id"`
	URL       string `json:"url"`
	Type      string `json:"type"`
	Cached    bool   `json:"cached"`
	CreatedAt string `json:"created_at"`
}

// Init opens the SQLite database and creates the required tables.
// dbPath defaults to ./data/pancy.db if SQLITE_PATH env var is not set.
func Init() error {
	dbPath := os.Getenv("SQLITE_PATH")
	if dbPath == "" {
		dbPath = "./data/pancy.db"
	}

	if err := os.MkdirAll("./data", 0755); err != nil {
		return err
	}

	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	// Enable WAL mode for better concurrent reads
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS screenshot_history (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    TEXT    NOT NULL,
		url        TEXT    NOT NULL,
		type       TEXT    NOT NULL,
		cached     INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_history_user_id ON screenshot_history(user_id);

	CREATE TABLE IF NOT EXISTS webhook_registrations (
		user_id     TEXT PRIMARY KEY,
		webhook_url TEXT NOT NULL,
		created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(schema)
	return err
}

// Close closes the database connection gracefully.
func Close() {
	if db != nil {
		_ = db.Close()
	}
}

// ───────────────────────────────────────────────────────────────────────────────
// History
// ───────────────────────────────────────────────────────────────────────────────

// AddHistory records a screenshot event. Anonymous requests (empty userID) are skipped.
func AddHistory(userID, url, screenshotType string, cached bool) error {
	if userID == "" {
		return nil
	}
	cachedInt := 0
	if cached {
		cachedInt = 1
	}
	_, err := db.Exec(
		`INSERT INTO screenshot_history (user_id, url, type, cached) VALUES (?, ?, ?, ?)`,
		userID, url, screenshotType, cachedInt,
	)
	return err
}

// GetHistory returns the last `limit` history entries for a user, newest first.
func GetHistory(userID string, limit int) ([]HistoryEntry, error) {
	rows, err := db.Query(
		`SELECT id, user_id, url, type, cached, created_at
		 FROM screenshot_history
		 WHERE user_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var cachedInt int
		if err := rows.Scan(&e.ID, &e.UserID, &e.URL, &e.Type, &cachedInt, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Cached = cachedInt == 1
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []HistoryEntry{}
	}
	return entries, nil
}

// ───────────────────────────────────────────────────────────────────────────────
// Webhook Registrations
// ───────────────────────────────────────────────────────────────────────────────

// RegisterWebhook registers or updates a webhook URL for a given user_id.
func RegisterWebhook(userID, webhookURL string) error {
	_, err := db.Exec(
		`INSERT INTO webhook_registrations (user_id, webhook_url)
		 VALUES (?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   webhook_url = excluded.webhook_url,
		   created_at  = CURRENT_TIMESTAMP`,
		userID, webhookURL,
	)
	return err
}

// UnregisterWebhook removes the webhook registration for a user.
func UnregisterWebhook(userID string) error {
	_, err := db.Exec(`DELETE FROM webhook_registrations WHERE user_id = ?`, userID)
	return err
}

// GetWebhook returns the registered webhook URL for a user, or false if none.
func GetWebhook(userID string) (string, bool) {
	var webhookURL string
	err := db.QueryRow(
		`SELECT webhook_url FROM webhook_registrations WHERE user_id = ?`, userID,
	).Scan(&webhookURL)
	if err != nil {
		return "", false
	}
	return webhookURL, true
}

// DeliverWebhook sends a JSON payload to a webhook URL asynchronously,
// retrying up to 3 times with exponential backoff (2s, 4s, 8s).
func DeliverWebhook(webhookURL string, payload any) {
	go func() {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Printf("[Webhook] Error serializando payload: %v", err)
			return
		}

		for attempt := 1; attempt <= 3; attempt++ {
			resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(data))
			if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				resp.Body.Close()
				log.Printf("[Webhook] Entregado a %s (intento %d)", webhookURL, attempt)
				return
			}
			if resp != nil {
				resp.Body.Close()
			}
			log.Printf("[Webhook] Intento %d fallido para %s: %v", attempt, webhookURL, err)
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
		}
		log.Printf("[Webhook] Falló la entrega a %s después de 3 intentos", webhookURL)
	}()
}
