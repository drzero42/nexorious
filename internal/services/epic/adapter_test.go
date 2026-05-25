package epic

import (
	"context"
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// fakeEpicClient satisfies clientInterface without invoking legendary.
type fakeEpicClient struct {
	configured       bool
	restoreErr       error
	getLibraryErr    error
	captureSnapshot  map[string]string
	captureErr       error
	libraryBatches   [][]ExternalGameEntry

	restoreCalled    bool
	getLibraryCalled bool
	captureCalled    bool
	restoredSnapshot map[string]string
}

func (f *fakeEpicClient) Configured() bool { return f.configured }

func (f *fakeEpicClient) RestoreSnapshot(_ string, snapshot map[string]string) error {
	f.restoreCalled = true
	f.restoredSnapshot = snapshot
	return f.restoreErr
}

func (f *fakeEpicClient) GetLibrary(_ context.Context, _ string, onBatch func([]ExternalGameEntry) error) error {
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

func (f *fakeEpicClient) CaptureSnapshot(_ string) (map[string]string, error) {
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
