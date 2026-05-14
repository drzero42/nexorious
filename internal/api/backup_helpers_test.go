package api

// Internal tests for unexported backup cron helpers.

import (
	"testing"
)

// ---------------------------------------------------------------------------
// parseCronToSchedule
// ---------------------------------------------------------------------------

func TestParseCronToSchedule_EmptyString(t *testing.T) {
	schedule, scheduleTime, scheduleDay := parseCronToSchedule("")
	if schedule != "manual" {
		t.Errorf("schedule = %q, want 'manual'", schedule)
	}
	if scheduleTime != "00:00" {
		t.Errorf("scheduleTime = %q, want '00:00'", scheduleTime)
	}
	if scheduleDay != 0 {
		t.Errorf("scheduleDay = %d, want 0", scheduleDay)
	}
}

func TestParseCronToSchedule_Daily(t *testing.T) {
	// "30 14 * * *" → daily at 14:30
	schedule, scheduleTime, scheduleDay := parseCronToSchedule("30 14 * * *")
	if schedule != "daily" {
		t.Errorf("schedule = %q, want 'daily'", schedule)
	}
	if scheduleTime != "14:30" {
		t.Errorf("scheduleTime = %q, want '14:30'", scheduleTime)
	}
	if scheduleDay != 0 {
		t.Errorf("scheduleDay = %d, want 0", scheduleDay)
	}
}

func TestParseCronToSchedule_Weekly(t *testing.T) {
	// "0 2 * * 0" → weekly, Sunday (cron 0 = Sun → frontend day = (0+6)%7 = 6)
	schedule, scheduleTime, scheduleDay := parseCronToSchedule("0 2 * * 0")
	if schedule != "weekly" {
		t.Errorf("schedule = %q, want 'weekly'", schedule)
	}
	if scheduleTime != "02:00" {
		t.Errorf("scheduleTime = %q, want '02:00'", scheduleTime)
	}
	if scheduleDay != 6 {
		t.Errorf("scheduleDay = %d, want 6 (Sunday → frontend 6)", scheduleDay)
	}
}

func TestParseCronToSchedule_Weekly_Monday(t *testing.T) {
	// "0 3 * * 1" → weekly, Monday (cron 1 = Mon → frontend day = (1+6)%7 = 0)
	schedule, _, scheduleDay := parseCronToSchedule("0 3 * * 1")
	if schedule != "weekly" {
		t.Errorf("schedule = %q, want 'weekly'", schedule)
	}
	if scheduleDay != 0 {
		t.Errorf("scheduleDay = %d, want 0 (Monday → frontend 0)", scheduleDay)
	}
}

func TestParseCronToSchedule_WrongFieldCount(t *testing.T) {
	// Cron with wrong number of fields → falls back to "manual"
	schedule, _, _ := parseCronToSchedule("30 14 * *") // only 4 fields
	if schedule != "manual" {
		t.Errorf("expected 'manual' for malformed cron, got %q", schedule)
	}
}

// ---------------------------------------------------------------------------
// buildCronFromSchedule
// ---------------------------------------------------------------------------

func TestBuildCronFromSchedule_Manual(t *testing.T) {
	cron, err := buildCronFromSchedule("manual", "00:00", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cron != "" {
		t.Errorf("expected empty cron for manual, got %q", cron)
	}
}

func TestBuildCronFromSchedule_Daily(t *testing.T) {
	cron, err := buildCronFromSchedule("daily", "14:30", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cron != "30 14 * * *" {
		t.Errorf("cron = %q, want '30 14 * * *'", cron)
	}
}

func TestBuildCronFromSchedule_Weekly_Monday(t *testing.T) {
	// Frontend day 0 = Monday → cron day = (0+1)%7 = 1
	cron, err := buildCronFromSchedule("weekly", "02:00", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cron != "0 2 * * 1" {
		t.Errorf("cron = %q, want '0 2 * * 1'", cron)
	}
}

func TestBuildCronFromSchedule_Weekly_Sunday(t *testing.T) {
	// Frontend day 6 = Sunday → cron day = (6+1)%7 = 0
	cron, err := buildCronFromSchedule("weekly", "03:15", 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cron != "15 3 * * 0" {
		t.Errorf("cron = %q, want '15 3 * * 0'", cron)
	}
}

func TestBuildCronFromSchedule_InvalidTime_NoColon(t *testing.T) {
	_, err := buildCronFromSchedule("daily", "1430", 0)
	if err == nil {
		t.Error("expected error for time without colon")
	}
}

func TestBuildCronFromSchedule_InvalidTime_BadHour(t *testing.T) {
	_, err := buildCronFromSchedule("daily", "25:00", 0)
	if err == nil {
		t.Error("expected error for hour > 23")
	}
}

func TestBuildCronFromSchedule_InvalidTime_BadMinute(t *testing.T) {
	_, err := buildCronFromSchedule("daily", "12:60", 0)
	if err == nil {
		t.Error("expected error for minute > 59")
	}
}

func TestBuildCronFromSchedule_Weekly_InvalidDay(t *testing.T) {
	_, err := buildCronFromSchedule("weekly", "12:00", 7) // day > 6
	if err == nil {
		t.Error("expected error for schedule_day > 6")
	}
}

func TestBuildCronFromSchedule_Weekly_NegativeDay(t *testing.T) {
	_, err := buildCronFromSchedule("weekly", "12:00", -1)
	if err == nil {
		t.Error("expected error for schedule_day < 0")
	}
}

func TestBuildCronFromSchedule_UnknownSchedule(t *testing.T) {
	_, err := buildCronFromSchedule("biweekly", "12:00", 0)
	if err == nil {
		t.Error("expected error for unknown schedule")
	}
}

func TestBuildCronFromSchedule_InvalidTimeNonNumeric(t *testing.T) {
	_, err := buildCronFromSchedule("daily", "xx:30", 0)
	if err == nil {
		t.Error("expected error for non-numeric hour")
	}
}
