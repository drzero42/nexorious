package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func statsServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user-games/stats", func(w http.ResponseWriter, _ *http.Request) {
		avg := 7.5
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_games":        342,
			"completion_stats":   map[string]int{"not_started": 100, "completed": 50},
			"ownership_stats":    map[string]int{"owned": 340, "borrowed": 2},
			"platform_stats":     map[string]int{"PC (Windows)": 200},
			"genre_stats":        map[string]int{"RPG": 120},
			"pile_of_shame":      100,
			"completion_rate":    41.2,
			"average_rating":     avg,
			"total_hours_played": 1280.5,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGameStatsTable(t *testing.T) {
	srv := statsServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "stats"})
	if err := root.Execute(); err != nil {
		t.Fatalf("stats: %v\n%s", err, out.String())
	}
	for _, want := range []string{"342", "1280.5", "41.2", "7.5", "not_started", "100", "owned", "340", "PC (Windows)", "200", "RPG", "120"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("stats table missing %q:\n%s", want, out.String())
		}
	}
}

func TestGameStatsJSON(t *testing.T) {
	srv := statsServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "stats", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("stats --json: %v\n%s", err, out.String())
	}
	var s map[string]any
	if err := json.Unmarshal(out.Bytes(), &s); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if s["total_games"].(float64) != 342 || s["completion_rate"].(float64) != 41.2 {
		t.Fatalf("json = %+v", s)
	}
}

func TestGameStatsQuiet(t *testing.T) {
	srv := statsServer(t)
	seedProfile(t, srv.URL)

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"game", "stats", "-q"})
	if err := root.Execute(); err != nil {
		t.Fatalf("stats -q: %v\n%s", err, out.String())
	}
	if got := bytes.TrimSpace(out.Bytes()); string(got) != "342" {
		t.Fatalf("quiet = %q, want 342", got)
	}
}
