package api

// Internal tests for unexported backup cron helpers.

import (
	"testing"
)

// ---------------------------------------------------------------------------
// parseCronToSchedule
// ---------------------------------------------------------------------------

func TestParseCronToSchedule(t *testing.T) {
	tests := []struct {
		name         string
		cron         string
		wantSchedule string
		wantTime     string
		wantDay      int
	}{
		{
			name:         "empty string falls back to manual",
			cron:         "",
			wantSchedule: "manual",
			wantTime:     "00:00",
			wantDay:      0,
		},
		{
			name:         "daily at 14:30",
			cron:         "30 14 * * *",
			wantSchedule: "daily",
			wantTime:     "14:30",
			wantDay:      0,
		},
		{
			// cron 0 = Sun → frontend day = (0+6)%7 = 6
			name:         "weekly Sunday",
			cron:         "0 2 * * 0",
			wantSchedule: "weekly",
			wantTime:     "02:00",
			wantDay:      6,
		},
		{
			// cron 1 = Mon → frontend day = (1+6)%7 = 0
			name:         "weekly Monday",
			cron:         "0 3 * * 1",
			wantSchedule: "weekly",
			wantTime:     "03:00",
			wantDay:      0,
		},
		{
			// Cron with wrong number of fields → falls back to "manual"
			name:         "wrong field count falls back to manual",
			cron:         "30 14 * *", // only 4 fields
			wantSchedule: "manual",
			wantTime:     "00:00",
			wantDay:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule, scheduleTime, scheduleDay := parseCronToSchedule(tt.cron)
			if schedule != tt.wantSchedule {
				t.Errorf("schedule = %q, want %q", schedule, tt.wantSchedule)
			}
			if scheduleTime != tt.wantTime {
				t.Errorf("scheduleTime = %q, want %q", scheduleTime, tt.wantTime)
			}
			if scheduleDay != tt.wantDay {
				t.Errorf("scheduleDay = %d, want %d", scheduleDay, tt.wantDay)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildCronFromSchedule
// ---------------------------------------------------------------------------

func TestBuildCronFromSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		time     string
		day      int
		wantCron string
		wantErr  bool
	}{
		{
			name:     "manual yields empty cron",
			schedule: "manual",
			time:     "00:00",
			day:      0,
			wantCron: "",
		},
		{
			name:     "daily at 14:30",
			schedule: "daily",
			time:     "14:30",
			day:      0,
			wantCron: "30 14 * * *",
		},
		{
			// Frontend day 0 = Monday → cron day = (0+1)%7 = 1
			name:     "weekly Monday",
			schedule: "weekly",
			time:     "02:00",
			day:      0,
			wantCron: "0 2 * * 1",
		},
		{
			// Frontend day 6 = Sunday → cron day = (6+1)%7 = 0
			name:     "weekly Sunday",
			schedule: "weekly",
			time:     "03:15",
			day:      6,
			wantCron: "15 3 * * 0",
		},
		{
			name:     "invalid time without colon",
			schedule: "daily",
			time:     "1430",
			day:      0,
			wantErr:  true,
		},
		{
			name:     "invalid time hour > 23",
			schedule: "daily",
			time:     "25:00",
			day:      0,
			wantErr:  true,
		},
		{
			name:     "invalid time minute > 59",
			schedule: "daily",
			time:     "12:60",
			day:      0,
			wantErr:  true,
		},
		{
			name:     "weekly day > 6",
			schedule: "weekly",
			time:     "12:00",
			day:      7,
			wantErr:  true,
		},
		{
			name:     "weekly negative day",
			schedule: "weekly",
			time:     "12:00",
			day:      -1,
			wantErr:  true,
		},
		{
			name:     "unknown schedule",
			schedule: "biweekly",
			time:     "12:00",
			day:      0,
			wantErr:  true,
		},
		{
			name:     "invalid time non-numeric hour",
			schedule: "daily",
			time:     "xx:30",
			day:      0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cron, err := buildCronFromSchedule(tt.schedule, tt.time, tt.day)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (cron=%q)", cron)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cron != tt.wantCron {
				t.Errorf("cron = %q, want %q", cron, tt.wantCron)
			}
		})
	}
}
