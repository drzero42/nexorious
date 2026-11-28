package storefront

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The guards of ADR 0010: this package is the only non-test Go code that
// names the stored sign-in's column, and it links none of the HTTP layer or
// the Job process.

const modulePath = "github.com/drzero42/nexorious"

// credentialsColumn matches the column name, not longer names that start
// with it.
var credentialsColumn = regexp.MustCompile(`\bstorefront_credentials\b`)

// columnExceptions are the functions outside this package allowed to name
// the column, keyed by package import path and function name.
//
// flagCredentialsError sets credentials_error only while the stored sign-in
// is still the one the sync loaded, comparing the column in its UPDATE. It is
// a health write and belongs with storefront health (#1410).
var columnExceptions = map[[2]string]bool{
	{modulePath + "/internal/worker/tasks", "flagCredentialsError"}: true,
}

// columnLiterals returns each string literal in the Go source src that names
// the column, with the function it is in ("" at package level). Struct tags
// are skipped: they name the model field, which is not guarded.
func columnLiterals(t *testing.T, fset *token.FileSet, name string, src any) []columnUse {
	t.Helper()
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	tags := map[*ast.BasicLit]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if field, ok := n.(*ast.Field); ok && field.Tag != nil {
			tags[field.Tag] = true
		}
		return true
	})
	var found []columnUse
	scan := func(fn string, root ast.Node) {
		ast.Inspect(root, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || tags[lit] {
				return true
			}
			if s, err := strconv.Unquote(lit.Value); err == nil && credentialsColumn.MatchString(s) {
				found = append(found, columnUse{pos: fset.Position(lit.Pos()).String(), fn: fn})
			}
			return true
		})
	}
	for _, decl := range f.Decls {
		fn := ""
		if d, ok := decl.(*ast.FuncDecl); ok {
			fn = d.Name.Name
		}
		scan(fn, decl)
	}
	return found
}

type columnUse struct {
	pos string
	fn  string
}

func goTool(t *testing.T) string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	return gobin
}

func goList(t *testing.T, args ...string) string {
	t.Helper()
	gobin := goTool(t)
	root, err := exec.Command(gobin, "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	cmd := exec.Command(gobin, append([]string{"list"}, args...)...)
	cmd.Dir = strings.TrimSpace(string(root))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// TestOnlyThisPackageNamesTheCredentialsColumn is the ownership guard: no
// non-test Go file outside this package has a string literal naming
// user_sync_configs.storefront_credentials, apart from the functions in
// columnExceptions. It does not cover reads of the model field
// StorefrontCredentials, test fixtures, migrations, or deletes of whole
// user_sync_configs rows. The files come from go list, not a directory walk,
// so other branches' sources under .worktrees/ are not scanned.
func TestOnlyThisPackageNamesTheCredentialsColumn(t *testing.T) {
	out := goList(t, "-f",
		`{{.ImportPath}}{{"\t"}}{{.Dir}}{{range .GoFiles}}{{"\t"}}{{.}}{{end}}{{range .CgoFiles}}{{"\t"}}{{.}}{{end}}{{range .IgnoredGoFiles}}{{"\t"}}{{.}}{{end}}`,
		"./...")
	self := modulePath + "/internal/storefront"
	fset := token.NewFileSet()
	scanned := 0
	usedExceptions := map[[2]string]bool{}
	for line := range strings.Lines(out) {
		fields := strings.Split(strings.TrimRight(line, "\n"), "\t")
		if len(fields) < 2 || fields[0] == self {
			continue
		}
		for _, name := range fields[2:] {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			scanned++
			for _, use := range columnLiterals(t, fset, filepath.Join(fields[1], name), nil) {
				key := [2]string{fields[0], use.fn}
				if columnExceptions[key] {
					usedExceptions[key] = true
					continue
				}
				t.Errorf("%s: names storefront_credentials; read and write the stored sign-in through internal/storefront (ADR 0010)", use.pos)
			}
		}
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d files; go list output looks wrong:\n%s", scanned, out)
	}
	for key := range columnExceptions {
		if !usedExceptions[key] {
			t.Errorf("exception %s.%s no longer names the column; remove it from columnExceptions", key[0], key[1])
		}
	}
}

// TestColumnScan pins what the ownership guard catches, so a scan that never
// fires cannot pass silently.
func TestColumnScan(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want []string // the functions of the uses found
	}{
		{"raw select", "func f() { db.NewRaw(`SELECT storefront_credentials FROM user_sync_configs`) }", []string{"f"}},
		{"where clause", `func g() { q.Where("storefront_credentials IS NULL") }`, []string{"g"}},
		{"package level", `const q = "UPDATE user_sync_configs SET storefront_credentials = ?"`, []string{""}},
		{"struct tag", "type M struct { C *string `bun:\"storefront_credentials\"` }", nil},
		{"comment", "// storefront_credentials\nfunc f() {}", nil},
		{"longer name", `func f() { q.Where("storefront_credentials_error") }`, nil},
		{"other column", `func f() { q.Where("credentials_error = true") }`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, use := range columnLiterals(t, token.NewFileSet(), "x.go", "package p\n\n"+tc.src+"\n") {
				got = append(got, use.fn)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("uses in %v, want %v", got, tc.want)
			}
		})
	}
}

// TestImportsNeitherHTTPNorJobs keeps the package below the layers that call
// it: it links neither internal/api, internal/worker/tasks nor
// internal/joblifecycle.
func TestImportsNeitherHTTPNorJobs(t *testing.T) {
	out := goList(t, "-deps", "-f", "{{.ImportPath}}", modulePath+"/internal/storefront")
	deps := strings.Fields(out)
	if !slices.Contains(deps, modulePath+"/internal/crypto") {
		t.Fatalf("deps lack internal/crypto; go list output looks wrong:\n%s", out)
	}
	for _, banned := range []string{"/internal/api", "/internal/worker/tasks", "/internal/joblifecycle"} {
		if slices.Contains(deps, modulePath+banned) {
			t.Errorf("internal/storefront links %s%s", modulePath, banned)
		}
	}
}
