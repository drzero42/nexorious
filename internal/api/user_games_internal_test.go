package api

import (
	"testing"

	"github.com/drzero42/nexorious/internal/db/models"
)

func TestToUserGamePlatformResponse_Achievements(t *testing.T) {
	u, total := 7, 10
	resp := toUserGamePlatformResponse(models.UserGamePlatform{
		ID:                   "p1",
		AchievementsUnlocked: &u,
		AchievementsTotal:    &total,
	})
	if resp.AchievementsUnlocked == nil || *resp.AchievementsUnlocked != 7 ||
		resp.AchievementsTotal == nil || *resp.AchievementsTotal != 10 {
		t.Fatalf("got %v/%v, want 7/10", resp.AchievementsUnlocked, resp.AchievementsTotal)
	}
}
