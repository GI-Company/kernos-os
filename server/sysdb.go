package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
)

var GlobalSysDB *SysDB
var JWTSecret []byte

type SysDB struct {
	DB *sql.DB
}

// InitSysDB initializes the persistent configuration, session, and audit database.
func InitSysDB() *SysDB {
	home, _ := os.UserHomeDir()
	dbDir := filepath.Join(home, ".kernos")
	os.MkdirAll(dbDir, 0755)
	dbPath := filepath.Join(dbDir, "sys.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		slog.Error("[SysDB] Failed to open config database", "error", err)
		os.Exit(1)
	}

	createTablesSQL := `
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT
	);
	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		expires_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		topic TEXT,
		identity TEXT,
		payload TEXT
	);
	CREATE TABLE IF NOT EXISTS sys_history (
		id TEXT PRIMARY KEY,
		category TEXT,
		target TEXT,
		prev_data TEXT,
		timestamp DATETIME
	);
	`

	if _, err := db.Exec(createTablesSQL); err != nil {
		slog.Error("[SysDB] Failed to create tables", "error", err)
		os.Exit(1)
	}

	sysDB := &SysDB{DB: db}

	// 1. JWT Secret Init
	jwtSec := sysDB.GetConfig("jwt_secret")
	if jwtSec == "" {
		secBytes := make([]byte, 32)
		rand.Read(secBytes)
		jwtSec = hex.EncodeToString(secBytes)
		sysDB.SetConfig("jwt_secret", jwtSec)
	}
	JWTSecret = []byte(jwtSec)

	// 2. Persistent Root AuthToken Init
	authToken := sysDB.GetConfig("root_auth_token")
	if authToken == "" {
		tokenBytes := make([]byte, 32)
		rand.Read(tokenBytes)
		authToken = hex.EncodeToString(tokenBytes)
		sysDB.SetConfig("root_auth_token", authToken)
	}
	AuthToken = authToken

	GlobalSysDB = sysDB
	slog.Info("[SysDB] Database initialized successfully", "path", dbPath)
	return sysDB
}

func (s *SysDB) GetConfig(key string) string {
	var val string
	err := s.DB.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&val)
	if err != nil {
		return ""
	}
	return val
}

func (s *SysDB) SetConfig(key, value string) error {
	_, err := s.DB.Exec("INSERT INTO config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	return err
}

func (s *SysDB) LogAudit(topic, identity, payload string) {
	_, err := s.DB.Exec("INSERT INTO audit_log (timestamp, topic, identity, payload) VALUES (?, ?, ?, ?)", time.Now(), topic, identity, payload)
	if err != nil {
		slog.Error("[SysDB] Failed to write to audit log", "error", err)
	}
}

func (s *SysDB) CreateSessionToken() (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour * 30)), // 30 day session
	})
	signedBlock, err := token.SignedString(JWTSecret)
	if err != nil {
		return "", err
	}
	_, err = s.DB.Exec("INSERT INTO sessions (token, expires_at) VALUES (?, ?)", signedBlock, time.Now().Add(24*time.Hour*30))
	return signedBlock, err
}

func (s *SysDB) ValidateSession(signedToken string) bool {
	var expires time.Time
	err := s.DB.QueryRow("SELECT expires_at FROM sessions WHERE token = ?", signedToken).Scan(&expires)
	if err != nil || time.Now().After(expires) {
		return false
	}
	token, _ := jwt.Parse(signedToken, func(token *jwt.Token) (interface{}, error) {
		return JWTSecret, nil
	})
	return token != nil && token.Valid
}
