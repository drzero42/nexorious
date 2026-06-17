package usergame

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/drzero42/nexorious/internal/db/models"
)

// ResolveOrCreateTag returns the id of the caller's tag named `name`, matching
// case-insensitively, creating the definition (with `color`) if it does not
// exist. Accepts bun.IDB so it runs inside a caller's transaction.
func ResolveOrCreateTag(ctx context.Context, db bun.IDB, userID, name string, color *string) (string, error) {
	var tag models.Tag
	err := db.NewSelect().Model(&tag).
		Where("user_id = ? AND LOWER(name) = LOWER(?)", userID, name).
		Scan(ctx)
	if err == nil {
		return tag.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("select tag: %w", err)
	}

	now := time.Now().UTC()
	tag = models.Tag{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      name,
		Color:     color,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err = db.NewInsert().Model(&tag).Exec(ctx); err != nil {
		return "", fmt.Errorf("insert tag: %w", err)
	}
	return tag.ID, nil
}

// ReplaceTags sets the complete tag set on a user game to `names`. It resolves
// or creates each name within the caller's tags, then reconciles
// user_game_tags: inserting missing links and deleting links no longer present.
// Names are trimmed and de-duplicated case-insensitively; an empty slice clears
// all tags. Accepts bun.IDB so it runs inside a caller's transaction.
func ReplaceTags(ctx context.Context, db bun.IDB, userGameID, userID string, names []string) error {
	desired := map[string]bool{} // tag id -> wanted
	seen := map[string]bool{}    // lower(name) -> resolved
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		tagID, err := ResolveOrCreateTag(ctx, db, userID, name, nil)
		if err != nil {
			return err
		}
		desired[tagID] = true
	}

	var existing []models.UserGameTag
	if err := db.NewSelect().Model(&existing).
		Where("user_game_id = ?", userGameID).Scan(ctx); err != nil {
		return fmt.Errorf("select existing tags: %w", err)
	}
	existingIDs := map[string]bool{}
	for _, ugt := range existing {
		existingIDs[ugt.TagID] = true
	}

	var toDelete []string
	for id := range existingIDs {
		if !desired[id] {
			toDelete = append(toDelete, id)
		}
	}
	if len(toDelete) > 0 {
		if _, err := db.NewDelete().Model((*models.UserGameTag)(nil)).
			Where("user_game_id = ?", userGameID).
			Where("tag_id IN (?)", bun.List(toDelete)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete tag links: %w", err)
		}
	}

	now := time.Now().UTC()
	for id := range desired {
		if existingIDs[id] {
			continue
		}
		ugt := &models.UserGameTag{
			ID:         uuid.NewString(),
			UserGameID: userGameID,
			TagID:      id,
			CreatedAt:  now,
		}
		if _, err := db.NewInsert().Model(ugt).Exec(ctx); err != nil {
			return fmt.Errorf("insert tag link: %w", err)
		}
	}
	return nil
}
