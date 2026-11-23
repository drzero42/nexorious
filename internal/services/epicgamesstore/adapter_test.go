package epicgamesstore

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// fakeEpicClient satisfies clientInterface without invoking legendary.
type fakeEpicClient struct {
	configured      bool
	restoreErr      error
	getLibraryErr   error
	captureSnapshot map[string]string
	captureErr      error
	libraryBatches  [][]ExternalGameEntry

	openErr error

	restoreCalled        bool
	getLibraryCalled     bool
	captureCalled        bool
	closed               bool
	persistedBeforeClose bool
	restoredSnapshot     map[string]string
}

func (f *fakeEpicClient) Configured() bool { return f.configured }

// newSession hands back the fake itself as the session.
func (f *fakeEpicClient) newSession() (librarySession, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	return f, nil
}

func (f *fakeEpicClient) Close() error {
	f.closed = true
	return nil
}

func (f *fakeEpicClient) RestoreSnapshot(snapshot map[string]string) error {
	f.restoreCalled = true
	f.restoredSnapshot = snapshot
	return f.restoreErr
}

func (f *fakeEpicClient) GetLibrary(_ context.Context, onBatch func([]ExternalGameEntry) error) error {
	f.getLibraryCalled = true
	if f.getLibraryErr != nil {
		return f.getLibraryErr
	}
	for _, batch := range f.libraryBatches {
		if err := onBatch(batch); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeEpicClient) CaptureSnapshot() (map[string]string, error) {
	f.captureCalled = true
	return f.captureSnapshot, f.captureErr
}

func TestEpicAdapter_NotConfigured_ReturnsError(t *testing.T) {
	fake := &fakeEpicClient{configured: false}
	a := NewAdapter(fake, "user1", map[string]string{"k": "v"}, nil)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if err == nil {
		t.Fatal("expected error when legendary not configured, got nil")
	}
	if fake.restoreCalled || fake.getLibraryCalled || fake.captureCalled {
		t.Error("expected no client calls when not configured")
	}
}

func TestEpicAdapter_RestoresSnapshotBeforeFetch(t *testing.T) {
	snapshot := map[string]string{"user.json": `{"displayName":"Test"}`}
	fake := &fakeEpicClient{configured: true, captureSnapshot: map[string]string{}}
	a := NewAdapter(fake, "user1", snapshot, nil)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.restoreCalled {
		t.Error("expected RestoreSnapshot to be called")
	}
	if fake.restoredSnapshot["user.json"] != snapshot["user.json"] {
		t.Errorf("snapshot mismatch: got %v, want %v", fake.restoredSnapshot, snapshot)
	}
}

func TestEpicAdapter_PersistsNewSnapshotAfterSuccess(t *testing.T) {
	newSnapshot := map[string]string{"user.json": `{"displayName":"Updated"}`}
	fake := &fakeEpicClient{configured: true, captureSnapshot: newSnapshot}

	var capturedSnapshot map[string]string
	onSnapshot := func(s map[string]string) error {
		capturedSnapshot = s
		return nil
	}
	a := NewAdapter(fake, "user1", map[string]string{}, onSnapshot)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSnapshot == nil {
		t.Fatal("expected onSnapshot to be called")
	}
	if capturedSnapshot["user.json"] != newSnapshot["user.json"] {
		t.Errorf("snapshot content mismatch: got %v, want %v", capturedSnapshot, newSnapshot)
	}
}

func TestEpicAdapter_PersistsSnapshotEvenOnFetchError(t *testing.T) {
	newSnapshot := map[string]string{"user.json": `{"displayName":"Updated"}`}
	fetchErr := errors.New("library fetch failed")
	fake := &fakeEpicClient{
		configured:      true,
		getLibraryErr:   fetchErr,
		captureSnapshot: newSnapshot,
	}
	var onSnapshotCalled bool
	onSnapshot := func(map[string]string) error {
		onSnapshotCalled = true
		return nil
	}
	a := NewAdapter(fake, "user1", map[string]string{}, onSnapshot)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, fetchErr) {
		t.Errorf("expected fetchErr, got %v", err)
	}
	if !onSnapshotCalled {
		t.Error("expected onSnapshot to be called even on fetch error")
	}
}

func TestEpicAdapter_SkipsPersistWhenSnapshotEmpty(t *testing.T) {
	fake := &fakeEpicClient{configured: true, captureSnapshot: map[string]string{}}
	var onSnapshotCalled bool
	onSnapshot := func(map[string]string) error {
		onSnapshotCalled = true
		return nil
	}
	a := NewAdapter(fake, "user1", map[string]string{}, onSnapshot)

	if err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if onSnapshotCalled {
		t.Error("expected onSnapshot NOT to be called when captured snapshot is empty")
	}
}

func TestEpicAdapter_LegendaryAuthFailure_ReturnsErrCredentials(t *testing.T) {
	fake := &fakeEpicClient{
		configured:      true,
		getLibraryErr:   ErrAuthFailed,
		captureSnapshot: map[string]string{},
	}
	a := NewAdapter(fake, "user1", map[string]string{}, nil)

	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected storefrontadapter.ErrCredentials, got %v", err)
	}
}

func TestEpicAdapter_MapsEntriesToStorefrontFormat(t *testing.T) {
	fake := &fakeEpicClient{
		configured: true,
		libraryBatches: [][]ExternalGameEntry{
			{{ExternalID: "fortnite", Title: "Fortnite", OwnershipStatus: "owned"}},
		},
		captureSnapshot: map[string]string{},
	}
	a := NewAdapter(fake, "user1", map[string]string{}, nil)

	var received []storefrontadapter.ExternalGameEntry
	if err := a.GetLibrary(context.Background(), 10, func(batch []storefrontadapter.ExternalGameEntry) error {
		received = append(received, batch...)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(received) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(received))
	}
	got := received[0]
	if got.ExternalID != "fortnite" || got.Title != "Fortnite" {
		t.Errorf("unexpected entry: %+v", got)
	}
	if len(got.Platforms) != 1 || got.Platforms[0] != "pc-windows" {
		t.Errorf("expected [pc-windows], got %v", got.Platforms)
	}
	if got.PlaytimeHours != 0 {
		t.Errorf("expected 0 playtime, got %v", got.PlaytimeHours)
	}
}

// TestEpicAdapter_ChunksLibraryIntoBatches covers the spec invariant from
// docs/sync.md § Epic: the adapter must re-chunk the client's single big
// batch into chunks of ≤10 before invoking the outer onBatch.
func TestEpicAdapter_ChunksLibraryIntoBatches(t *testing.T) {
	// Build one client-side batch of 25 entries.
	big := make([]ExternalGameEntry, 25)
	for i := range big {
		big[i] = ExternalGameEntry{
			ExternalID:      fmt.Sprintf("game-%02d", i),
			Title:           fmt.Sprintf("Game %02d", i),
			OwnershipStatus: "owned",
		}
	}
	fake := &fakeEpicClient{
		configured:      true,
		libraryBatches:  [][]ExternalGameEntry{big},
		captureSnapshot: map[string]string{},
	}
	a := NewAdapter(fake, "user1", map[string]string{}, nil)

	var receivedSizes []int
	if err := a.GetLibrary(context.Background(), 10, func(batch []storefrontadapter.ExternalGameEntry) error {
		receivedSizes = append(receivedSizes, len(batch))
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSizes := []int{10, 10, 5}
	if len(receivedSizes) != len(wantSizes) {
		t.Fatalf("expected %d outer onBatch calls, got %d (sizes=%v)", len(wantSizes), len(receivedSizes), receivedSizes)
	}
	for i, got := range receivedSizes {
		if got != wantSizes[i] {
			t.Errorf("batch %d size: want %d, got %d", i, wantSizes[i], got)
		}
	}
}

// The session directory holds the user's Epic credentials, so it must be
// removed after every sync — and only after the refreshed snapshot is saved.
func TestEpicAdapter_ClosesSessionAfterSync(t *testing.T) {
	cases := []struct {
		name string
		fake *fakeEpicClient
	}{
		{"success", &fakeEpicClient{configured: true, captureSnapshot: map[string]string{"user.json": "new"}}},
		{"fetch error", &fakeEpicClient{configured: true, getLibraryErr: errors.New("boom"), captureSnapshot: map[string]string{"user.json": "new"}}},
		{"auth failure", &fakeEpicClient{configured: true, getLibraryErr: ErrAuthFailed, captureSnapshot: map[string]string{"user.json": "new"}}},
		{"restore error", &fakeEpicClient{configured: true, restoreErr: errors.New("disk full")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := tc.fake
			onSnapshot := func(map[string]string) error {
				fake.persistedBeforeClose = !fake.closed
				return nil
			}
			a := NewAdapter(fake, "user1", map[string]string{"user.json": "old"}, onSnapshot)
			_ = a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
			if !fake.closed {
				t.Fatal("session was not closed")
			}
			if fake.captureCalled && !fake.persistedBeforeClose {
				t.Error("snapshot was persisted after the session was closed")
			}
		})
	}
}

func TestEpicAdapter_SessionOpenError(t *testing.T) {
	fake := &fakeEpicClient{configured: true, openErr: errors.New("read-only fs")}
	a := NewAdapter(fake, "user1", nil, nil)
	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if err == nil || fake.restoreCalled {
		t.Fatalf("want open error before restore; err=%v restoreCalled=%v", err, fake.restoreCalled)
	}
}
