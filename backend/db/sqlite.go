package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ConnectToSQLite initializes and returns a SQLite connection
func ConnectToSQLite(dbPath string) (*sql.DB, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for SQLite: %w", err)
	}
	// Open connection with extended query string parameters for better concurrency
	dsn := fmt.Sprintf("%s?_journal=WAL&_timeout=30000&_busy_timeout=30000", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// Set connection pool size - important for handling concurrent requests
	db.SetMaxOpenConns(15)                  // Allow up to 10 concurrent connections
	db.SetMaxIdleConns(10)                  // Keep up to 5 idle connections
	db.SetConnMaxLifetime(30 * time.Minute) // Recycle connections after 30 minutes

	// Set PRAGMA statements for better concurrent access
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=30000", // Increased to 30 seconds
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=10000", // Increased cache size
		"PRAGMA foreign_keys=ON",
		"PRAGMA temp_store=MEMORY",   // Use memory for temp storage
		"PRAGMA mmap_size=268435456", // Use memory mapping (256MB)
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return nil, fmt.Errorf("failed to set %s: %w", pragma, err)
		}
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping SQLite database: %w", err)
	}

	log.Println("Connected to SQLite database with optimized settings for concurrency")
	return db, nil
}

// InitializeSchema creates all the necessary tables if they don't exist
func InitializeSchema(db *sql.DB) error {
	// Create networks table
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS networks (
		id TEXT PRIMARY KEY,
		cidr TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("failed to create networks table: %w", err)
	}

	// Create devices table
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS devices (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		ipv4 TEXT NOT NULL,
		mac TEXT,
		vendor TEXT,
		status TEXT NOT NULL,
		network_id TEXT,
		hostname TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		last_seen_online_at TIMESTAMP,
		port_scan_started_at TIMESTAMP,
		port_scan_ended_at TIMESTAMP,
		FOREIGN KEY (network_id) REFERENCES networks(id)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create devices table: %w", err)
	}

	// Create unique index on ipv4 to prevent duplicate IP addresses
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_ipv4 ON devices(ipv4)`)
	if err != nil {
		return fmt.Errorf("failed to create unique index on devices.ipv4: %w", err)
	}

	// Create index on MAC address for faster lookups
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_devices_mac ON devices(mac)`)
	if err != nil {
		return fmt.Errorf("failed to create index on devices.mac: %w", err)
	}

	// Create index on network_id for faster network queries
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_devices_network_id ON devices(network_id)`)
	if err != nil {
		return fmt.Errorf("failed to create index on devices.network_id: %w", err)
	}

	// Create ports table
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS ports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		number TEXT NOT NULL,
		protocol TEXT NOT NULL,
		state TEXT NOT NULL,
		service TEXT NOT NULL,
		FOREIGN KEY (device_id) REFERENCES devices(id)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create ports table: %w", err)
	}

	// Create event_logs table
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS event_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		description TEXT NOT NULL,
		device_id TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("failed to create event_logs table: %w", err)
	}

	// Create system_status table
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS system_status (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		network_id TEXT,
		public_ip TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		FOREIGN KEY (network_id) REFERENCES networks(id)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create system_status table: %w", err)
	}

	// Create local_device table for system_status
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS local_devices (
		system_status_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		ipv4 TEXT NOT NULL,
		mac TEXT,
		vendor TEXT,
		status TEXT NOT NULL,
		hostname TEXT,
		PRIMARY KEY (system_status_id),
		FOREIGN KEY (system_status_id) REFERENCES system_status(id)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create local_devices table: %w", err)
	}

	log.Println("Database schema initialized successfully")
	return nil
}
