package backup

import "time"

const (
	ManifestVersion    = 1
	MaxManifestVersion = 1
)

// Manifest represents the manifest.json inside a backup archive.
type Manifest struct {
	Version           int       `json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	AppVersion        string    `json:"app_version"`
	MigrationVersion  string    `json:"migration_version"`
	BackupType        string    `json:"backup_type"`
	DatabaseFile      string    `json:"database_file"`
	DatabaseSizeBytes int64     `json:"database_size_bytes"`
	DatabaseChecksum  string    `json:"database_checksum"`
	CoverArtCount     int       `json:"cover_art_count"`
	CoverArtSizeBytes int64     `json:"cover_art_size_bytes"`
	CoverArtChecksum  string    `json:"cover_art_checksum"`
	StatsUsers        int       `json:"stats_users"`
	StatsGames        int       `json:"stats_games"`
	StatsTags         int       `json:"stats_tags"`
}

// BackupInfo is the summary returned by ListBackups.
type BackupInfo struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	BackupType string    `json:"backup_type"`
	SizeBytes  int64     `json:"size_bytes"`
	Stats      struct {
		Users int `json:"users"`
		Games int `json:"games"`
		Tags  int `json:"tags"`
	} `json:"stats"`
}
