package humble

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// fakeClient satisfies libraryClient for adapter tests.
type fakeClient struct {
	gamekeys  []string
	orders    map[string]*Order
	listErr   error
	orderErrs map[string]error
}

func (f *fakeClient) ListGamekeys(_ context.Context, _ string) ([]string, error) {
	return f.gamekeys, f.listErr
}

func (f *fakeClient) GetOrder(_ context.Context, _, gamekey string) (*Order, error) {
	if f.orderErrs != nil {
		if err := f.orderErrs[gamekey]; err != nil {
			return nil, err
		}
	}
	return f.orders[gamekey], nil
}

// gameDownload builds a qualifying game download for a platform.
func gameDownload(platform string) Download {
	return Download{
		Platform: platform,
		DownloadStruct: []DownloadStruct{{URL: struct {
			Web string `json:"web"`
		}{Web: "https://dl.example/file"}}},
	}
}

// collect runs the adapter over a single-order fake and returns all yielded entries.
func collect(t *testing.T, order *Order) []storefrontadapter.ExternalGameEntry {
	t.Helper()
	fc := &fakeClient{gamekeys: []string{"GK1"}, orders: map[string]*Order{"GK1": order}}
	a := NewAdapter(fc, "cookie")
	var got []storefrontadapter.ExternalGameEntry
	err := a.GetLibrary(context.Background(), 10, func(batch []storefrontadapter.ExternalGameEntry) error {
		got = append(got, batch...)
		return nil
	})
	if err != nil {
		t.Fatalf("GetLibrary error: %v", err)
	}
	return got
}

func TestGetLibrary_FilteringRule(t *testing.T) {
	tests := []struct {
		name    string
		sub     Subproduct
		include bool
	}{
		{
			name:    "windows game included",
			sub:     Subproduct{MachineName: "aquaria", HumanName: "Aquaria", Downloads: []Download{gameDownload("windows")}},
			include: true,
		},
		{
			name:    "android-only game included",
			sub:     Subproduct{MachineName: "aquaria_android", HumanName: "Aquaria", Downloads: []Download{gameDownload("android")}},
			include: true,
		},
		{
			name:    "ebook excluded",
			sub:     Subproduct{MachineName: "wog_ebook", HumanName: "WoG ebook", Downloads: []Download{gameDownload("ebook")}},
			include: false,
		},
		{
			name:    "audio excluded",
			sub:     Subproduct{MachineName: "ost", HumanName: "Soundtrack", Downloads: []Download{gameDownload("audio")}},
			include: false,
		},
		{
			name:    "video excluded",
			sub:     Subproduct{MachineName: "doc", HumanName: "Documentary", Downloads: []Download{gameDownload("video")}},
			include: false,
		},
		{
			name:    "asmjs excluded",
			sub:     Subproduct{MachineName: "webgame", HumanName: "Web Game", Downloads: []Download{gameDownload("asmjs")}},
			include: false,
		},
		{
			name:    "freegame_info stub excluded (empty downloads)",
			sub:     Subproduct{MachineName: "civ3_freegame_info", HumanName: "Civ III", Downloads: nil},
			include: false,
		},
		{
			name: "steam-key-only excluded (game-platform download but empty download_struct)",
			sub: Subproduct{MachineName: "abzu", HumanName: "ABZU", Downloads: []Download{
				{Platform: "windows", DownloadStruct: nil},
			}},
			include: false,
		},
		{
			name: "empty url.web excluded",
			sub: Subproduct{MachineName: "broken", HumanName: "Broken", Downloads: []Download{
				{Platform: "windows", DownloadStruct: []DownloadStruct{{}}},
			}},
			include: false,
		},
		{
			name:    "uplayclient launcher excluded despite real windows download",
			sub:     Subproduct{MachineName: "uplayclient", HumanName: "Uplay Client (will download latest version)", Downloads: []Download{gameDownload("windows")}},
			include: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collect(t, &Order{Gamekey: "GK1", Subproducts: []Subproduct{tt.sub}})
			if tt.include && len(got) != 1 {
				t.Fatalf("expected 1 entry, got %d: %+v", len(got), got)
			}
			if !tt.include && len(got) != 0 {
				t.Fatalf("expected 0 entries, got %d: %+v", len(got), got)
			}
		})
	}
}

func TestGetLibrary_EntryFields(t *testing.T) {
	got := collect(t, &Order{Gamekey: "GK1", Subproducts: []Subproduct{
		{MachineName: "aquaria", HumanName: "Aquaria", Downloads: []Download{
			gameDownload("windows"), gameDownload("mac"), gameDownload("linux"),
		}},
	}})
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	e := got[0]
	if e.ExternalID != "aquaria" || e.Title != "Aquaria" {
		t.Errorf("ExternalID/Title = %q/%q", e.ExternalID, e.Title)
	}
	if e.PlaytimeHours != 0 || e.OwnershipStatus != "owned" || e.IsSubscription {
		t.Errorf("unexpected fields: %+v", e)
	}
	platforms := append([]string(nil), e.Platforms...)
	sort.Strings(platforms)
	want := []string{"mac", "pc-linux", "pc-windows"}
	if len(platforms) != 3 || platforms[0] != want[0] || platforms[1] != want[1] || platforms[2] != want[2] {
		t.Errorf("platforms = %v, want %v", platforms, want)
	}
}

func TestGetLibrary_GameWithDirectDownloadAndSteamKeyIncluded(t *testing.T) {
	got := collect(t, &Order{Gamekey: "GK1", Subproducts: []Subproduct{
		{MachineName: "braid", HumanName: "Braid", Downloads: []Download{gameDownload("windows")}},
	}})
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
}

func TestGetLibrary_SeparatePCAndAndroidEditionsBothEmitted(t *testing.T) {
	got := collect(t, &Order{Gamekey: "GK1", Subproducts: []Subproduct{
		{MachineName: "aquaria", HumanName: "Aquaria", Downloads: []Download{gameDownload("windows")}},
		{MachineName: "aquaria_android", HumanName: "Aquaria", Downloads: []Download{gameDownload("android")}},
	}})
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].ExternalID != "aquaria" || got[1].ExternalID != "aquaria_android" {
		t.Errorf("external ids = %q, %q", got[0].ExternalID, got[1].ExternalID)
	}
}

func TestGetLibrary_SkipsFailingOrder(t *testing.T) {
	fc := &fakeClient{
		gamekeys: []string{"GK1", "GK2"},
		orders: map[string]*Order{
			"GK2": {Gamekey: "GK2", Subproducts: []Subproduct{
				{MachineName: "braid", HumanName: "Braid", Downloads: []Download{gameDownload("windows")}},
			}},
		},
		orderErrs: map[string]error{"GK1": errors.New("boom")},
	}
	a := NewAdapter(fc, "cookie")
	var got []storefrontadapter.ExternalGameEntry
	err := a.GetLibrary(context.Background(), 10, func(b []storefrontadapter.ExternalGameEntry) error {
		got = append(got, b...)
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error (bad order skipped), got %v", err)
	}
	if len(got) != 1 || got[0].ExternalID != "braid" {
		t.Errorf("expected only braid from GK2, got %+v", got)
	}
}

func TestGetLibrary_ListErrCredentialsWrapped(t *testing.T) {
	fc := &fakeClient{listErr: ErrCredentials}
	a := NewAdapter(fc, "cookie")
	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected storefrontadapter.ErrCredentials, got %v", err)
	}
}

func TestGetLibrary_OrderErrCredentialsWrapped(t *testing.T) {
	fc := &fakeClient{
		gamekeys:  []string{"GK1"},
		orderErrs: map[string]error{"GK1": ErrCredentials},
	}
	a := NewAdapter(fc, "cookie")
	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error { return nil })
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("expected storefrontadapter.ErrCredentials on per-order auth failure, got %v", err)
	}
}

func TestGetLibrary_BatchBoundaryPreservesAllEntries(t *testing.T) {
	order := &Order{Gamekey: "GK1", Subproducts: []Subproduct{
		{MachineName: "a", HumanName: "A", Downloads: []Download{gameDownload("windows")}},
		{MachineName: "b", HumanName: "B", Downloads: []Download{gameDownload("mac")}},
		{MachineName: "c", HumanName: "C", Downloads: []Download{gameDownload("linux")}},
	}}
	fc := &fakeClient{gamekeys: []string{"GK1"}, orders: map[string]*Order{"GK1": order}}
	a := NewAdapter(fc, "cookie")

	var batches [][]storefrontadapter.ExternalGameEntry
	var got []storefrontadapter.ExternalGameEntry
	// batchSize 1 forces a flush after every entry; retain each batch slice to
	// prove later batches don't corrupt earlier ones.
	err := a.GetLibrary(context.Background(), 1, func(batch []storefrontadapter.ExternalGameEntry) error {
		batches = append(batches, batch)
		got = append(got, batch...)
		return nil
	})
	if err != nil {
		t.Fatalf("GetLibrary error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(got), got)
	}
	wantIDs := []string{"a", "b", "c"}
	for i, id := range wantIDs {
		if got[i].ExternalID != id {
			t.Errorf("entry %d ExternalID = %q, want %q", i, got[i].ExternalID, id)
		}
	}
	// Each retained batch slice must still hold its original entry (no aliasing corruption).
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches at batchSize 1, got %d", len(batches))
	}
	for i, id := range wantIDs {
		if len(batches[i]) != 1 || batches[i][0].ExternalID != id {
			t.Errorf("retained batch %d = %+v, want single entry %q", i, batches[i], id)
		}
	}
}
