package api

import (
	"strings"
	"testing"

	"github.com/drzero42/nexorious/internal/db/models"
)

// The sync view's constant SQL fragments splice activeJobStatusList instead of
// binding models.ActiveJobStatuses; the two must list the same statuses.
func TestActiveJobStatusList(t *testing.T) {
	quoted := make([]string, 0, len(models.ActiveJobStatuses()))
	for _, s := range models.ActiveJobStatuses() {
		quoted = append(quoted, "'"+string(s)+"'")
	}
	if want := strings.Join(quoted, ", "); activeJobStatusList != want {
		t.Errorf("activeJobStatusList = %s, want %s", activeJobStatusList, want)
	}
}
