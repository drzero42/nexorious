package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// fastRestart shortens the restart wait for a test.
func fastRestart(t *testing.T, grace time.Duration) {
	t.Helper()
	oldGrace, oldPoll := restartGrace, restartPoll
	restartGrace, restartPoll = grace, 5*time.Millisecond
	t.Cleanup(func() { restartGrace, restartPoll = oldGrace, oldPoll })
}

// fakeRestarter plays a server that restarts after a restore: /health reports
// instance "old" until restart is called, then "new".
type fakeRestarter struct {
	mu        sync.Mutex
	id        string
	restoring bool
	polls     int
}

func newFakeRestarter() *fakeRestarter { return &fakeRestarter{id: "old"} }

func (f *fakeRestarter) health(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls++
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok", "instance_id": f.id, "restore_in_progress": f.restoring,
	})
}

func (f *fakeRestarter) set(id string, restoring bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.id, f.restoring = id, restoring
}

func (f *fakeRestarter) pollCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls
}

// answerAndRestart writes a restore response that says the server is
// restarting, then lets the new instance appear shortly afterwards.
func (f *fakeRestarter) answerAndRestart(w http.ResponseWriter, status int, body map[string]any) {
	body["restarting"] = true
	body["instance_id"] = "old"
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
	go func() {
		time.Sleep(20 * time.Millisecond)
		f.set("new", false)
	}()
}

func runRestoreAgainst(t *testing.T, restore http.HandlerFunc, f *fakeRestarter) (string, error) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", f.health)
	mux.HandleFunc("/api/auth/setup/restore/disk", restore)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := cliclient.New(srv.URL)
	var out bytes.Buffer
	err := runRestore(t.Context(), &out, c, func(ctx context.Context) (*cliclient.RestoreResult, error) {
		return c.SetupRestoreFromDisk(ctx, "b.tar.gz")
	})
	return out.String(), err
}

func TestRunRestore(t *testing.T) {
	t.Run("success waits for the new instance", func(t *testing.T) {
		fastRestart(t, time.Second)
		f := newFakeRestarter()
		out, err := runRestoreAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			f.answerAndRestart(w, http.StatusOK, map[string]any{"success": true, "message": "Restore completed from bk-1."})
		}, f)
		if err != nil {
			t.Fatalf("runRestore: %v\n%s", err, out)
		}
		for _, want := range []string{"Restore completed from bk-1.", "Waiting for the server to restart", "The server is back."} {
			if !strings.Contains(out, want) {
				t.Errorf("output %q lacks %q", out, want)
			}
		}
	})

	t.Run("a failure that restarts waits, then reports the failure", func(t *testing.T) {
		fastRestart(t, time.Second)
		f := newFakeRestarter()
		out, err := runRestoreAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			f.answerAndRestart(w, http.StatusInternalServerError, map[string]any{
				"error": "restore failed: bad dump (rolled back to pre-restore backup p-1)", "rolled_back": true,
			})
		}, f)
		if err == nil || !strings.Contains(err.Error(), "rolled back") {
			t.Fatalf("err = %v, want the server's failure", err)
		}
		if !strings.Contains(out, "The server is back.") {
			t.Errorf("output %q: did not wait for the restart", out)
		}
	})

	t.Run("a refusal that does not restart returns at once", func(t *testing.T) {
		fastRestart(t, time.Second)
		f := newFakeRestarter()
		out, err := runRestoreAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"restarting": false, "instance_id": "old", "error": "A backup or restore operation is already in progress",
			})
		}, f)
		var he *cliclient.HTTPError
		if !errors.As(err, &he) || he.Status != http.StatusConflict {
			t.Fatalf("err = %v, want the 409", err)
		}
		if strings.Contains(out, "Waiting") || f.pollCount() != 1 {
			t.Errorf("waited after a refusal (output %q, %d health polls)", out, f.pollCount())
		}
	})

	t.Run("no answer at all still waits for the restart", func(t *testing.T) {
		fastRestart(t, time.Second)
		f := newFakeRestarter()
		out, err := runRestoreAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			go func() { time.Sleep(20 * time.Millisecond); f.set("new", false) }()
			conn, _, herr := http.NewResponseController(w).Hijack()
			if herr != nil {
				t.Errorf("hijack: %v", herr)
				return
			}
			_ = conn.Close() // the connection drops: no response
		}, f)
		if err == nil || !strings.Contains(err.Error(), "outcome is unknown") {
			t.Fatalf("err = %v, want an unknown outcome", err)
		}
		if !strings.Contains(out, "The server is back.") {
			t.Errorf("output %q: did not wait for the restart", out)
		}
	})

	t.Run("a plain 500 without a restore body waits", func(t *testing.T) {
		fastRestart(t, time.Second)
		f := newFakeRestarter()
		out, err := runRestoreAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			go func() { time.Sleep(20 * time.Millisecond); f.set("new", false) }()
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}, f)
		if err == nil || !strings.Contains(out, "The server is back.") {
			t.Fatalf("err = %v, output %q: want the 500 reported after the restart", err, out)
		}
	})

	t.Run("a restore still in progress outlasts the grace period", func(t *testing.T) {
		fastRestart(t, 50*time.Millisecond)
		f := newFakeRestarter()
		out, err := runRestoreAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			// The answer is lost (as behind a gateway that gave up) while the
			// old server goes on restoring for four grace periods.
			f.set("old", true)
			go func() {
				time.Sleep(200 * time.Millisecond)
				f.set("new", false)
			}()
			w.WriteHeader(http.StatusGatewayTimeout)
		}, f)
		if !strings.Contains(out, "The server is back.") {
			t.Fatalf("gave up while the restore was in progress: err = %v, output %q", err, out)
		}
	})

	t.Run("no new instance within the grace period is an error", func(t *testing.T) {
		fastRestart(t, 50*time.Millisecond)
		f := newFakeRestarter()
		_, err := runRestoreAgainst(t, func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true, "message": "done", "restarting": true, "instance_id": "old",
			})
		}, f)
		if !errors.Is(err, cliclient.ErrNoRestart) {
			t.Fatalf("err = %v, want ErrNoRestart", err)
		}
	})
}
