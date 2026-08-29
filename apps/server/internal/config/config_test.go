package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultsAndEnv(t *testing.T) {
	t.Setenv("MPACK_LISTEN_ADDR", "127.0.0.1:19999")
	t.Setenv("MPACK_DOWNLOAD_CONCURRENCY", "8")
	c, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != "127.0.0.1:19999" || c.DownloadConcurrency != 8 {
		t.Fatalf("config overrides not applied: %+v", c)
	}
}

func TestValidateLANRequiresToken(t *testing.T) {
	c := Default()
	c.AllowLAN = true
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "startup_token") {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestLoadFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte("listen_addr = \"127.0.0.1:1\"\ndownload_concurrency = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != "127.0.0.1:1" || c.DownloadConcurrency != 2 {
		t.Fatalf("got %+v", c)
	}
}
