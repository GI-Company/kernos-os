package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ═══════════════════════════════════════════════════════════════
// Kernos User Account System — Persistent Per-User Memory
// ═══════════════════════════════════════════════════════════════
//
// Features:
//   • SQLite-backed user storage (~/.kernos/users.db)
//   • bcrypt password hashing (cost 12)
//   • Per-user data directories (~/.kernos/users/{username}/)
//   • Isolated memory.db, vectors.db, and config per user
//   • Guest mode (ephemeral, wiped on logout)
// ═══════════════════════════════════════════════════════════════

type UserAccount struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	PasswordHash string `json:"-"` // Never serialize
	AvatarURL    string `json:"avatar_url"`
	Role         string `json:"role"` // admin, user, guest
	CreatedAt    string `json:"created_at"`
	LastLogin    string `json:"last_login"`
}

type UserAccountDB struct {
	db      *sql.DB
	dataDir string // ~/.kernos
}

// InitUserAccountDB opens or creates the users database
func InitUserAccountDB() (*UserAccountDB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot find home directory: %w", err)
	}

	dataDir := filepath.Join(home, ".kernos")
	os.MkdirAll(dataDir, 0755)

	dbPath := filepath.Join(dataDir, "users.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open users.db: %w", err)
	}

	udb := &UserAccountDB{db: db, dataDir: dataDir}

	if err := udb.migrate(); err != nil {
		return nil, fmt.Errorf("user db migration failed: %w", err)
	}

	log.Printf("[UserDB] ✅ User account database initialized at %s", dbPath)
	return udb, nil
}

func (udb *UserAccountDB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		password_hash TEXT NOT NULL,
		avatar_url TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT 'user',
		created_at TEXT NOT NULL,
		last_login TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token TEXT UNIQUE NOT NULL,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);`
	_, err := udb.db.Exec(schema)
	return err
}

// CreateUser registers a new user with a bcrypt-hashed password
func (udb *UserAccountDB) CreateUser(username, password, displayName, role string) (*UserAccount, error) {
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}
	if len(password) < 4 {
		return nil, fmt.Errorf("password must be at least 4 characters")
	}

	// Hash password with bcrypt (cost 12)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return nil, fmt.Errorf("bcrypt hash failed: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	avatarURL := fmt.Sprintf("https://api.dicebear.com/7.x/initials/svg?seed=%s", username)

	if displayName == "" {
		displayName = username
	}
	if role == "" {
		role = "user"
	}

	result, err := udb.db.Exec(
		`INSERT INTO users (username, display_name, password_hash, avatar_url, role, created_at, last_login) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		username, displayName, string(hash), avatarURL, role, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, _ := result.LastInsertId()

	// Create per-user data directory
	userDir := udb.GetUserDataDir(username)
	os.MkdirAll(userDir, 0755)
	log.Printf("[UserDB] 📁 Created user data directory: %s", userDir)

	user := &UserAccount{
		ID:          id,
		Username:    username,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
		Role:        role,
		CreatedAt:   now,
		LastLogin:   now,
	}

	log.Printf("[UserDB] ✅ Created user: %s (role: %s)", username, role)
	return user, nil
}

// Authenticate validates credentials and returns the user
func (udb *UserAccountDB) Authenticate(username, password string) (*UserAccount, error) {
	var user UserAccount
	var hash string

	err := udb.db.QueryRow(
		`SELECT id, username, display_name, password_hash, avatar_url, role, created_at, last_login FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &hash, &user.AvatarURL, &user.Role, &user.CreatedAt, &user.LastLogin)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	// Update last login
	now := time.Now().Format(time.RFC3339)
	udb.db.Exec(`UPDATE users SET last_login = ? WHERE id = ?`, now, user.ID)
	user.LastLogin = now

	log.Printf("[UserDB] 🔓 User authenticated: %s", username)
	return &user, nil
}

// ListUsers returns all registered users (for login screen)
func (udb *UserAccountDB) ListUsers() ([]UserAccount, error) {
	rows, err := udb.db.Query(
		`SELECT id, username, display_name, avatar_url, role, created_at, last_login FROM users ORDER BY last_login DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserAccount
	for rows.Next() {
		var u UserAccount
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.Role, &u.CreatedAt, &u.LastLogin); err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, nil
}

// HasUsers checks if any users exist (for first-run detection)
func (udb *UserAccountDB) HasUsers() bool {
	var count int
	udb.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count > 0
}

// GetUserDataDir returns the per-user data directory path
func (udb *UserAccountDB) GetUserDataDir(username string) string {
	return filepath.Join(udb.dataDir, "users", username)
}

// GetUserMemoryDBPath returns the path to the user's memory database
func (udb *UserAccountDB) GetUserMemoryDBPath(username string) string {
	return filepath.Join(udb.GetUserDataDir(username), "memory.db")
}

// GetUserVectorDBPath returns the path to the user's vector database
func (udb *UserAccountDB) GetUserVectorDBPath(username string) string {
	return filepath.Join(udb.GetUserDataDir(username), "vectors.db")
}

// DeleteUser removes a user and their data directory
func (udb *UserAccountDB) DeleteUser(username string) error {
	_, err := udb.db.Exec(`DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE username = ?)`, username)
	if err != nil {
		return err
	}
	_, err = udb.db.Exec(`DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return err
	}

	// Remove user data directory
	userDir := udb.GetUserDataDir(username)
	os.RemoveAll(userDir)

	log.Printf("[UserDB] ❌ Deleted user: %s", username)
	return nil
}

// Close cleans up the database connection
func (udb *UserAccountDB) Close() error {
	return udb.db.Close()
}
