package models

import (
	"time"

	"github.com/uptrace/bun"
)

// BackupConfig is the singleton (id=1) backup schedule configuration.
type BackupConfig struct {
	bun.BaseModel `bun:"table:backup_config"`

	ID             int       `bun:"id,pk"              json:"id"`
	ScheduleCron   string    `bun:"schedule_cron,notnull" json:"schedule_cron"`
	RetentionMode  string    `bun:"retention_mode,notnull" json:"retention_mode"`
	RetentionValue int        `bun:"retention_value,notnull" json:"retention_value"`
	LastBackupAt   *time.Time `bun:"last_backup_at"          json:"last_backup_at"`
	CreatedAt      time.Time  `bun:"created_at,notnull"      json:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at,notnull" json:"updated_at"`
}
