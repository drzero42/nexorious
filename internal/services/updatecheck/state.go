package updatecheck

import "sync"

// State holds the most recent successful update-check result. Written by the
// periodic worker, read by the /api/version handler. Safe for concurrent use.
// Zero value (via NewState) means "no check has succeeded yet".
type State struct {
	mu            sync.RWMutex
	latestVersion string // normalized, no leading "v" (e.g. "0.10.0")
	releaseURL    string
}

// NewState returns an empty State.
func NewState() *State {
	return &State{}
}

// Set stores the latest known release. Called only on successful fetches, so
// a failed run leaves the last good value in place.
func (s *State) Set(latestVersion, releaseURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latestVersion = latestVersion
	s.releaseURL = releaseURL
}

// Latest returns the stored latest version (no leading "v") and release URL.
// Both are empty until the first successful check.
func (s *State) Latest() (latestVersion, releaseURL string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestVersion, s.releaseURL
}
