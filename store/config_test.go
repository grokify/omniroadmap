package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFrom_Missing(t *testing.T) {
	cfg, err := LoadConfigFrom(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("LoadConfigFrom(missing): %v", err)
	}
	if cfg.Port != 0 || cfg.DSN != "" {
		t.Errorf("missing config should be empty, got %+v", cfg)
	}
}

func TestLoadConfigFrom_Malformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigFrom(path); err == nil {
		t.Error("LoadConfigFrom(malformed) = nil error, want parse error")
	}
}

func TestConfig_SaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := (&Config{Port: 13308, DataDir: "/tmp/x"}).SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	cfg, err := LoadConfigFrom(path)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if cfg.Port != 13308 || cfg.DataDir != "/tmp/x" {
		t.Errorf("round trip = %+v, want port 13308, data dir /tmp/x", cfg)
	}
}

func TestConfig_Resolve(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		flagDSN     string
		flagPort    int
		wantDSN     string
		wantPort    int
		wantPortSrc string
	}{
		{
			name:        "all defaults",
			wantDSN:     "root:@tcp(127.0.0.1:13307)/omniroadmap",
			wantPort:    13307,
			wantPortSrc: "default",
		},
		{
			name:        "config port drives DSN",
			cfg:         Config{Port: 13308},
			wantDSN:     "root:@tcp(127.0.0.1:13308)/omniroadmap",
			wantPort:    13308,
			wantPortSrc: "config",
		},
		{
			name:        "flag port beats config port",
			cfg:         Config{Port: 13308},
			flagPort:    13309,
			wantDSN:     "root:@tcp(127.0.0.1:13309)/omniroadmap",
			wantPort:    13309,
			wantPortSrc: "flag",
		},
		{
			name:        "config database name",
			cfg:         Config{Port: 13308, Database: "roadmaps"},
			wantDSN:     "root:@tcp(127.0.0.1:13308)/roadmaps",
			wantPort:    13308,
			wantPortSrc: "config",
		},
		{
			name:        "explicit DSN wins and its port drives the server",
			cfg:         Config{Port: 13308},
			flagDSN:     "root:@tcp(127.0.0.1:13310)/other",
			wantDSN:     "root:@tcp(127.0.0.1:13310)/other",
			wantPort:    13310,
			wantPortSrc: "dsn",
		},
		{
			name:        "config DSN port beats config port field",
			cfg:         Config{Port: 13308, DSN: "root:@tcp(127.0.0.1:13311)/omniroadmap"},
			wantDSN:     "root:@tcp(127.0.0.1:13311)/omniroadmap",
			wantPort:    13311,
			wantPortSrc: "dsn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, err := tt.cfg.Resolve(tt.flagDSN, tt.flagPort, "")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if st.DSN != tt.wantDSN {
				t.Errorf("DSN = %q, want %q", st.DSN, tt.wantDSN)
			}
			if st.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", st.Port, tt.wantPort)
			}
			if st.PortSource != tt.wantPortSrc {
				t.Errorf("PortSource = %q, want %q", st.PortSource, tt.wantPortSrc)
			}
		})
	}
}

func TestConfig_Resolve_EnvVars(t *testing.T) {
	t.Setenv(EnvPort, "13312")
	st, err := (&Config{Port: 13308}).Resolve("", 0, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if st.Port != 13312 || st.PortSource != "env" {
		t.Errorf("Port = %d (from %s), want 13312 from env", st.Port, st.PortSource)
	}

	t.Setenv(EnvPort, "not-a-number")
	if _, err := (&Config{}).Resolve("", 0, ""); err == nil {
		t.Error("Resolve with invalid OMNIROADMAP_PORT: err = nil, want error")
	}
}

func TestPortFromDSN(t *testing.T) {
	tests := []struct {
		dsn    string
		want   int
		wantOK bool
	}{
		{"root:@tcp(127.0.0.1:13307)/omniroadmap", 13307, true},
		{"user:pw@tcp(dbhost:3306)/db?parseTime=true", 3306, true},
		{"root:@unix(/tmp/mysql.sock)/db", 0, false},
		{"root:@tcp(127.0.0.1)/db", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := PortFromDSN(tt.dsn)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("PortFromDSN(%q) = (%d, %v), want (%d, %v)", tt.dsn, got, ok, tt.want, tt.wantOK)
		}
	}
}
