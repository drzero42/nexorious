package importsource_test

import (
	"errors"
	"testing"

	"github.com/drzero42/nexorious/internal/db/models"
	"github.com/drzero42/nexorious/internal/services/importmodel"
	"github.com/drzero42/nexorious/internal/services/importsource"
)

func TestLookup_Darkadia(t *testing.T) {
	src, ok := importsource.Lookup(models.JobSourceDarkadia)
	if !ok {
		t.Fatal("darkadia not registered")
	}
	if src.DisplayName != "Darkadia" {
		t.Errorf("DisplayName = %q, want Darkadia", src.DisplayName)
	}
	if src.Mapper == nil {
		t.Error("Mapper is nil")
	}
}

func TestLookup_Unknown(t *testing.T) {
	if _, ok := importsource.Lookup("nope"); ok {
		t.Error("unknown slug reported as registered")
	}
}

func TestIsRegistered(t *testing.T) {
	if !importsource.IsRegistered(models.JobSourceDarkadia) {
		t.Error("darkadia should be registered")
	}
	if importsource.IsRegistered(models.JobSourceNexorious) {
		t.Error("nexorious is not a mapper-based migration source")
	}
}

func TestAll_IncludesDarkadia(t *testing.T) {
	found := false
	for _, s := range importsource.All() {
		if s.Slug == models.JobSourceDarkadia {
			found = true
		}
	}
	if !found {
		t.Error("All() omits darkadia")
	}
}

func TestDarkadiaMapper_RejectsWrongFile(t *testing.T) {
	src, _ := importsource.Lookup(models.JobSourceDarkadia)
	_, err := src.Mapper.Parse([]byte("not,a,darkadia,file\n1,2,3,4\n"))
	if !errors.Is(err, importmodel.ErrInvalidSignature) {
		t.Errorf("err = %v, want wrapping ErrInvalidSignature", err)
	}
}
