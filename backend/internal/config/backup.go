package config

import (
	"fmt"
	"time"
)

const DefaultBackupTimeoutSeconds = 30 * 60

// BackupConfig controls the full dump, compression and upload time budget.
type BackupConfig struct {
	TimeoutSeconds int `mapstructure:"timeout_seconds"`
}

func (c BackupConfig) Timeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return time.Duration(DefaultBackupTimeoutSeconds) * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

func (c BackupConfig) Validate() error {
	if c.TimeoutSeconds < 0 || c.TimeoutSeconds > 86400 {
		return fmt.Errorf("backup.timeout_seconds must be between 0 and 86400 (0 uses the default)")
	}
	return nil
}
