package amazongames

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/drzero42/nexorious/internal/services/storefrontadapter"
)

// fakeLibraryClient satisfies LibraryClient without invoking nile.
type fakeLibraryClient struct {
	configured   bool
	entitlements []Entitlement
	refreshed    json.RawMessage
	err          error
	gotUserData  json.RawMessage
}

func (f *fakeLibraryClient) Configured() bool { return f.configured }

func (f *fakeLibraryClient) Library(_ context.Context, userData json.RawMessage) ([]Entitlement, json.RawMessage, error) {
	f.gotUserData = userData
	return f.entitlements, f.refreshed, f.err
}

// collect runs GetLibrary and returns every batch it delivered.
func collect(t *testing.T, a storefrontadapter.Adapter, batchSize int) ([][]storefrontadapter.ExternalGameEntry, error) {
	t.Helper()
	var batches [][]storefrontadapter.ExternalGameEntry
	err := a.GetLibrary(context.Background(), batchSize, func(b []storefrontadapter.ExternalGameEntry) error {
		batches = append(batches, append([]storefrontadapter.ExternalGameEntry(nil), b...))
		return nil
	})
	return batches, err
}

func ids(batches [][]storefrontadapter.ExternalGameEntry) [][]string {
	out := make([][]string, 0, len(batches))
	for _, b := range batches {
		row := make([]string, 0, len(b))
		for _, e := range b {
			row = append(row, e.ExternalID)
		}
		out = append(out, row)
	}
	return out
}

func entitlementsN(n int) []Entitlement {
	out := make([]Entitlement, n)
	for i := range out {
		out[i] = Entitlement{ProductID: fmt.Sprintf("p%d", i+1), Title: fmt.Sprintf("Game %d", i+1)}
	}
	return out
}

func TestAdapter_NotConfigured(t *testing.T) {
	a := NewAdapter(&fakeLibraryClient{configured: false}, json.RawMessage(`{}`), nil)
	if _, err := collect(t, a, 10); err == nil {
		t.Fatal("expected an error when nile is not installed")
	}
}

func TestAdapter_Batches(t *testing.T) {
	cases := []struct {
		name string
		n    int
		want [][]string
	}{
		{"zero", 0, [][]string{}},
		{"one", 1, [][]string{{"p1"}}},
		{"many across batches", 5, [][]string{{"p1", "p2"}, {"p3", "p4"}, {"p5"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAdapter(&fakeLibraryClient{configured: true, entitlements: entitlementsN(tc.n)}, json.RawMessage(`{}`), nil)
			batches, err := collect(t, a, 2)
			if err != nil {
				t.Fatalf("GetLibrary: %v", err)
			}
			if got := ids(batches); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("batches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAdapter_MapsEntitlement(t *testing.T) {
	a := NewAdapter(&fakeLibraryClient{configured: true, entitlements: []Entitlement{{ProductID: "amzn1.adg.product.9", Title: "Some Game"}}}, json.RawMessage(`{}`), nil)
	batches, err := collect(t, a, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := storefrontadapter.ExternalGameEntry{
		ExternalID: "amzn1.adg.product.9", Title: "Some Game",
		Platforms: []string{"pc-windows"}, OwnershipStatus: "owned",
	}
	if len(batches) != 1 || len(batches[0]) != 1 || !reflect.DeepEqual(batches[0][0], want) {
		t.Errorf("batches = %+v, want [[%+v]]", batches, want)
	}
}

func TestAdapter_EmptyTitleNamesTheProduct(t *testing.T) {
	a := NewAdapter(&fakeLibraryClient{configured: true, entitlements: []Entitlement{{ProductID: "p1", Title: "  "}}}, json.RawMessage(`{}`), nil)
	batches, err := collect(t, a, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := batches[0][0].Title; got != "Amazon product p1" {
		t.Errorf("title = %q, want %q", got, "Amazon product p1")
	}
}

func TestAdapter_DuplicateProductOnce(t *testing.T) {
	ents := []Entitlement{{"p1", "Game"}, {"p2", "Other"}, {"p1", "Game again"}}
	a := NewAdapter(&fakeLibraryClient{configured: true, entitlements: ents}, json.RawMessage(`{}`), nil)
	batches, err := collect(t, a, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(batches); !reflect.DeepEqual(got, [][]string{{"p1", "p2"}}) {
		t.Errorf("batches = %v", got)
	}
	if batches[0][0].Title != "Game" {
		t.Errorf("first entitlement's title should win, got %q", batches[0][0].Title)
	}
}

// A failure anywhere in the read returns before any batch, so the worker's
// removal sweep never runs over a partial library.
func TestAdapter_FailureDeliversNoBatch(t *testing.T) {
	fake := &fakeLibraryClient{configured: true, err: errors.New("amazon: nile library sync failed: KeyError")}
	batches, err := collect(t, NewAdapter(fake, json.RawMessage(`{}`), nil), 10)
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Errorf("err = %v, must not be a credentials error", err)
	}
	if len(batches) != 0 {
		t.Errorf("delivered %d batches before failing", len(batches))
	}
}

func TestAdapter_AuthFailureIsCredentialsError(t *testing.T) {
	fake := &fakeLibraryClient{configured: true, err: ErrAuthFailed}
	batches, err := collect(t, NewAdapter(fake, json.RawMessage(`{}`), nil), 10)
	if !errors.Is(err, storefrontadapter.ErrCredentials) {
		t.Fatalf("err = %v, want ErrCredentials", err)
	}
	if len(batches) != 0 {
		t.Errorf("delivered %d batches", len(batches))
	}
}

func TestAdapter_RefreshedUserDataPersistedBeforeBatches(t *testing.T) {
	var events []string
	fake := &fakeLibraryClient{configured: true, entitlements: entitlementsN(1), refreshed: json.RawMessage(`{"new":true}`)}
	a := NewAdapter(fake, json.RawMessage(`{"old":true}`), func(u json.RawMessage) error {
		events = append(events, "persist "+string(u))
		return nil
	})
	err := a.GetLibrary(context.Background(), 10, func([]storefrontadapter.ExternalGameEntry) error {
		events = append(events, "batch")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{`persist {"new":true}`, "batch"}; !reflect.DeepEqual(events, want) {
		t.Errorf("events = %v, want %v", events, want)
	}
	if string(fake.gotUserData) != `{"old":true}` {
		t.Errorf("client got user data %s", fake.gotUserData)
	}
}

func TestAdapter_RefreshedUserDataPersistedEvenOnFailure(t *testing.T) {
	persisted := 0
	fake := &fakeLibraryClient{configured: true, refreshed: json.RawMessage(`{"new":true}`), err: errors.New("boom")}
	a := NewAdapter(fake, json.RawMessage(`{}`), func(json.RawMessage) error { persisted++; return nil })
	if _, err := collect(t, a, 10); err == nil {
		t.Fatal("expected an error")
	}
	if persisted != 1 {
		t.Errorf("persisted %d times, want 1", persisted)
	}
}

func TestAdapter_PersistFailureDoesNotFailSync(t *testing.T) {
	fake := &fakeLibraryClient{configured: true, entitlements: entitlementsN(1), refreshed: json.RawMessage(`{}`)}
	a := NewAdapter(fake, json.RawMessage(`{}`), func(json.RawMessage) error { return errors.New("db down") })
	if _, err := collect(t, a, 10); err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}
}
