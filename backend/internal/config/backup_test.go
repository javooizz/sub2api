package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadBackupTimeout(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		env  string
		want time.Duration
	}{
		{name: "default", want: 30 * time.Minute},
		{name: "large database config", yaml: "backup:\n  timeout_seconds: 21600\n", want: 6 * time.Hour},
		{name: "environment overrides YAML", yaml: "backup:\n  timeout_seconds: 21600\n", env: "7200", want: 2 * time.Hour},
		{name: "zero uses default", env: "0", want: 30 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("BACKUP_TIMEOUT_SECONDS", tc.env)
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.yaml), 0o600))
			t.Setenv("CONFIG_FILE", path)
			cfg, err := Load()
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Backup.Timeout())
		})
	}
}

func TestLoadBackupTimeoutRejectsInvalidBounds(t *testing.T) {
	for _, value := range []string{"-1", "86401"} {
		t.Run(value, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("BACKUP_TIMEOUT_SECONDS", value)
			_, err := Load()
			require.ErrorContains(t, err, "backup.timeout_seconds")
		})
	}
}
