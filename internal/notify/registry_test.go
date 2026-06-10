package notify

import "testing"

func TestRegistryHasExpectedTypes(t *testing.T) {
	want := []string{
		"sync.completed", "sync.completed_with_errors", "sync.failed",
		"sync.auth_expired", "sync.needs_review", "sync.diff",
		"import.completed", "import.failed", "export.completed", "export.failed",
		"admin.backup.completed", "admin.backup.failed",
		"admin.maintenance.completed", "admin.maintenance.failed",
		"admin.version.available",
	}
	for _, typ := range want {
		if _, ok := Meta(typ); !ok {
			t.Errorf("registry missing event type %q", typ)
		}
	}
}

func TestDefaultSubscriptionsAreFailuresOnly(t *testing.T) {
	defaults := DefaultSubscriptions()
	got := map[string]bool{}
	for _, d := range defaults {
		got[d] = true
	}
	// admin.version.available is the deliberate non-failure exception: it is default-on by design (issue #899).
	for _, typ := range []string{"sync.failed", "sync.auth_expired", "import.failed", "export.failed", "sync.completed_with_errors", "admin.backup.failed", "admin.maintenance.failed", "admin.version.available"} {
		if !got[typ] {
			t.Errorf("expected default-on for %q", typ)
		}
	}
	for _, typ := range []string{"sync.completed", "import.completed", "export.completed", "admin.backup.completed", "admin.maintenance.completed", "sync.needs_review", "sync.diff"} {
		if got[typ] {
			t.Errorf("expected default-off for %q", typ)
		}
	}
}

func TestIsAdminType(t *testing.T) {
	if !IsAdminType("admin.backup.failed") {
		t.Error("admin.backup.failed should be admin-scoped")
	}
	if IsAdminType("sync.failed") {
		t.Error("sync.failed should not be admin-scoped")
	}
	if IsAdminType("does.not.exist") {
		t.Error("unknown type should not be admin-scoped")
	}
}
