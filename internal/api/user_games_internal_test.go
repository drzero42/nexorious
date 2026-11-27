package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/enum"
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

func TestValidateOwnership(t *testing.T) {
	ptr := func(s enum.OwnershipStatus) *enum.OwnershipStatus { return &s }
	tests := []struct {
		name    string
		in      *enum.OwnershipStatus
		wantErr bool
	}{
		{"nil passes", nil, false},
		{"empty passes", ptr(""), false},
		{"valid passes", ptr(enum.OwnershipBorrowed), false},
		{"unknown is rejected", ptr("stolen"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOwnership(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateOwnership() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				return
			}
			var he *echo.HTTPError
			if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
				t.Fatalf("want a 400 *echo.HTTPError, got %#v", err)
			}
		})
	}
}
