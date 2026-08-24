package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config file names inside the data directory (DefaultDataDir).
// ConfigFileName is canonical; the dotted variant is also accepted.
const (
	ConfigFileName    = "config.json"
	altConfigFileName = ".config.json"
)

// Environment variables overriding config-file values (CLI flags override
// both): OMNIROADMAP_DSN, OMNIROADMAP_PORT, OMNIROADMAP_DATA_DIR.
const (
	EnvDSN     = "OMNIROADMAP_DSN"
	EnvPort    = "OMNIROADMAP_PORT"
	EnvDataDir = "OMNIROADMAP_DATA_DIR"
)

// Config is the persisted store configuration
// (~/.omniroadmap/config.json). All fields are optional; unset fields fall
// back to built-in defaults. Values are resolved in order:
// CLI flag > environment variable > config file > default.
//
// Running several Dolt-backed tools side by side (e.g. visionstudio on
// 13306) is the main use case: set Port (or a full DSN) to keep each
// tool's dolt sql-server on its own port.
type Config struct {
	// Port for the local dolt sql-server. Ignored when DSN is set — the
	// DSN's own port wins.
	Port int `json:"port,omitempty"`
	// Database is the Dolt database name (default "omniroadmap").
	Database string `json:"database,omitempty"`
	// DataDir is the Dolt data directory (default ~/.omniroadmap).
	DataDir string `json:"data_dir,omitempty"`
	// DSN is a full MySQL-wire DSN; when set it takes precedence over
	// Port/Database.
	DSN string `json:"dsn,omitempty"`
}

// Settings is a fully-resolved store configuration, with the source of
// each value ("flag", "env", "config", "dsn", or "default") for
// diagnostics.
type Settings struct {
	DSN     string
	Port    int
	DataDir string

	DSNSource     string
	PortSource    string
	DataDirSource string
}

// ConfigPath returns the config file path: <data dir>/config.json, or the
// .config.json variant when only that exists.
func ConfigPath() string {
	dir := DefaultDataDir()
	canonical := filepath.Join(dir, ConfigFileName)
	if _, err := os.Stat(canonical); err == nil {
		return canonical
	}
	alt := filepath.Join(dir, altConfigFileName)
	if _, err := os.Stat(alt); err == nil {
		return alt
	}
	return canonical
}

// LoadConfig reads the config file from ConfigPath. A missing file yields
// an empty Config (all defaults); a malformed file is an error.
func LoadConfig() (*Config, error) {
	return LoadConfigFrom(ConfigPath())
}

// LoadConfigFrom reads a Config from an explicit path, with the same
// missing-file behavior as LoadConfig.
func LoadConfigFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: operator-owned config path
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("store: reading config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("store: parsing config %s: %w", path, err)
	}
	return &cfg, nil
}

// Save writes the config to <data dir>/config.json.
func (c *Config) Save() error {
	return c.SaveTo(filepath.Join(DefaultDataDir(), ConfigFileName))
}

// SaveTo writes the config to an explicit path, creating parent
// directories as needed.
func (c *Config) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("store: creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("store: writing config %s: %w", path, err)
	}
	return nil
}

// Resolve merges CLI-flag overrides (zero value = not set) with
// environment variables, the config file, and built-in defaults into
// final Settings. When the DSN is given explicitly (flag, env, or
// config), its embedded port is authoritative — EnsureServer must watch
// the port clients actually connect to.
func (c *Config) Resolve(flagDSN string, flagPort int, flagDataDir string) (Settings, error) {
	s := Settings{}

	switch {
	case flagDataDir != "":
		s.DataDir, s.DataDirSource = flagDataDir, "flag"
	case os.Getenv(EnvDataDir) != "":
		s.DataDir, s.DataDirSource = os.Getenv(EnvDataDir), "env"
	case c.DataDir != "":
		s.DataDir, s.DataDirSource = c.DataDir, "config"
	default:
		s.DataDir, s.DataDirSource = DefaultDataDir(), "default"
	}

	database := c.Database
	if database == "" {
		database = DefaultDatabase
	}

	port, portSource := 0, ""
	switch {
	case flagPort != 0:
		port, portSource = flagPort, "flag"
	case os.Getenv(EnvPort) != "":
		p, err := strconv.Atoi(os.Getenv(EnvPort))
		if err != nil {
			return Settings{}, fmt.Errorf("store: %s=%q is not a port number: %w", EnvPort, os.Getenv(EnvPort), err)
		}
		port, portSource = p, "env"
	case c.Port != 0:
		port, portSource = c.Port, "config"
	}

	dsn, dsnSource := "", ""
	switch {
	case flagDSN != "":
		dsn, dsnSource = flagDSN, "flag"
	case os.Getenv(EnvDSN) != "":
		dsn, dsnSource = os.Getenv(EnvDSN), "env"
	case c.DSN != "":
		dsn, dsnSource = c.DSN, "config"
	}

	if dsn != "" {
		// Explicit DSN: its port is what clients connect to, so it drives
		// the server too.
		if p, ok := PortFromDSN(dsn); ok {
			port, portSource = p, "dsn"
		} else if port == 0 {
			port, portSource = DefaultPort, "default"
		}
	} else {
		if port == 0 {
			port, portSource = DefaultPort, "default"
		}
		dsn = DSNForPort(port, database)
		dsnSource = portSource
	}

	s.DSN, s.DSNSource = dsn, dsnSource
	s.Port, s.PortSource = port, portSource
	return s, nil
}

// DSNForPort builds the standard local DSN for a port and database name.
func DSNForPort(port int, database string) string {
	return fmt.Sprintf("root:@tcp(127.0.0.1:%d)/%s", port, database)
}

// PortFromDSN extracts the TCP port from a go-sql-driver DSN like
// "root:@tcp(127.0.0.1:13307)/omniroadmap".
func PortFromDSN(dsn string) (int, bool) {
	open := strings.Index(dsn, "tcp(")
	if open < 0 {
		return 0, false
	}
	rest := dsn[open+len("tcp("):]
	closeIdx := strings.Index(rest, ")")
	if closeIdx < 0 {
		return 0, false
	}
	hostPort := rest[:closeIdx]
	colon := strings.LastIndex(hostPort, ":")
	if colon < 0 {
		return 0, false
	}
	port, err := strconv.Atoi(hostPort[colon+1:])
	if err != nil || port <= 0 {
		return 0, false
	}
	return port, true
}
