package externalgame

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The guards of ADR 0009: this package is the only non-test Go code that
// writes external_games and external_game_platforms, and it links none of the
// Job process or the HTTP layer.

const modulePath = "github.com/drzero42/nexorious"

// sqlWriteTarget matches a SQL write whose target table is one of the two
// tables: the table directly after the keyword, allowing whitespace,
// newlines, ONLY, TABLE, an optional public. and quotes. TRUNCATE may list
// other tables first. The (?:"|\b) after the name keeps external_game_id and
// other longer names out.
var sqlWriteTarget = regexp.MustCompile(`(?is)\b(?:` +
	`UPDATE|INSERT\s+INTO|DELETE\s+FROM|MERGE\s+INTO|` +
	`TRUNCATE(?:\s+TABLE)?(?:\s+ONLY)?(?:\s+[\w."]+\s*,)*` +
	`)\s+(?:ONLY\s+)?(?:"?public"?\s*\.\s*)?"?(?:external_games|external_game_platforms)(?:"|\b)`)

// builderTable matches a Table/TableExpr/ModelTableExpr argument that names
// one of the two tables.
var builderTable = regexp.MustCompile(`(?i)^\s*(?:"?public"?\s*\.\s*)?"?(?:external_games|external_game_platforms)(?:"|\b)`)

var builderWriteStarts = map[string]bool{
	"NewUpdate": true, "NewInsert": true, "NewDelete": true, "NewMerge": true, "NewTruncateTable": true,
}

// writesInFile returns a description of each write to the two tables in the
// Go source src: SQL string literals (a + concatenation is joined first, with
// a placeholder for each operand that is not a literal) and query-builder
// write chains.
func writesInFile(t *testing.T, fset *token.FileSet, name string, src any) []string {
	t.Helper()
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var found []string
	report := func(pos token.Pos, what string) {
		found = append(found, fset.Position(pos).String()+": "+what)
	}

	var inspect func(n ast.Node) bool
	inspect = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BinaryExpr:
			if n.Op != token.ADD {
				return true
			}
			var parts []string
			var others []ast.Expr
			var flatten func(e ast.Expr)
			flatten = func(e ast.Expr) {
				e = ast.Unparen(e)
				if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.ADD {
					flatten(b.X)
					flatten(b.Y)
					return
				}
				if s, ok := stringLit(e); ok {
					parts = append(parts, s)
					return
				}
				parts = append(parts, " ? ")
				others = append(others, e)
			}
			flatten(n)
			if sql := strings.Join(parts, ""); sqlWriteTarget.MatchString(sql) {
				report(n.Pos(), "SQL write: "+oneLine(sql))
			}
			for _, e := range others {
				ast.Inspect(e, inspect)
			}
			return false
		case *ast.BasicLit:
			if s, ok := stringLit(n); ok && sqlWriteTarget.MatchString(s) {
				report(n.Pos(), "SQL write: "+oneLine(s))
			}
		case *ast.CallExpr:
			if what, ok := builderWrite(n); ok {
				report(n.Pos(), what)
				// The inner calls of the chain are the same write.
				for _, a := range n.Args {
					ast.Inspect(a, inspect)
				}
				return false
			}
		}
		return true
	}
	ast.Inspect(f, inspect)
	return found
}

// builderWrite reports whether call is the outermost call of a query-builder
// chain that starts with a write (NewUpdate, NewInsert, …) and names one of
// the two tables, through Model or a Table argument.
func builderWrite(call *ast.CallExpr) (string, bool) {
	var start string
	names := false
	for cur := call; cur != nil; {
		sel, ok := cur.Fun.(*ast.SelectorExpr)
		if !ok {
			break
		}
		switch sel.Sel.Name {
		case "Model":
			for _, a := range cur.Args {
				if mentionsModel(a) {
					names = true
				}
			}
		case "Table", "TableExpr", "ModelTableExpr":
			if len(cur.Args) > 0 {
				if s, ok := stringLit(cur.Args[0]); ok && builderTable.MatchString(s) {
					names = true
				}
			}
		}
		if builderWriteStarts[sel.Sel.Name] {
			start = sel.Sel.Name
		}
		inner, _ := ast.Unparen(sel.X).(*ast.CallExpr)
		cur = inner
	}
	if start == "" || !names {
		return "", false
	}
	return "query-builder write: " + start + "() on an External game table", true
}

func mentionsModel(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "models" &&
			(sel.Sel.Name == "ExternalGame" || sel.Sel.Name == "ExternalGamePlatform") {
			found = true
		}
		return !found
	})
	return found
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := ast.Unparen(e).(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 100 {
		s = s[:100] + "…"
	}
	return s
}

func goTool(t *testing.T) string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	return gobin
}

// TestOnlyThisPackageWritesExternalGames is the write guard: no non-test Go
// file outside this package writes external_games or external_game_platforms.
// The files come from go list, not a directory walk, so other branches'
// sources under .worktrees/ are not scanned. Files excluded by build
// constraints are scanned too.
func TestOnlyThisPackageWritesExternalGames(t *testing.T) {
	gobin := goTool(t)
	root, err := exec.Command(gobin, "list", "-m", "-f", "{{.Dir}}").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	cmd := exec.Command(gobin, "list", "-f",
		`{{.ImportPath}}{{"\t"}}{{.Dir}}{{range .GoFiles}}{{"\t"}}{{.}}{{end}}{{range .CgoFiles}}{{"\t"}}{{.}}{{end}}{{range .IgnoredGoFiles}}{{"\t"}}{{.}}{{end}}`,
		"./...")
	cmd.Dir = strings.TrimSpace(string(root))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list ./...: %v\n%s", err, out)
	}

	self := modulePath + "/internal/externalgame"
	fset := token.NewFileSet()
	scanned := 0
	var violations []string
	for line := range strings.Lines(string(out)) {
		fields := strings.Split(strings.TrimRight(line, "\n"), "\t")
		if len(fields) < 2 || fields[0] == self {
			continue
		}
		for _, name := range fields[2:] {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			scanned++
			violations = append(violations, writesInFile(t, fset, filepath.Join(fields[1], name), nil)...)
		}
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d files; go list output looks wrong:\n%s", scanned, out)
	}
	for _, v := range violations {
		t.Errorf("%s\n\twrites to external_games and external_game_platforms belong in internal/externalgame (ADR 0009)", v)
	}
}

// TestWriteGuardMatcher pins what the write guard catches, so a matcher that
// never fires cannot pass silently.
func TestWriteGuardMatcher(t *testing.T) {
	cases := []struct {
		name  string
		sql   string
		write bool
	}{
		{"store link update", "UPDATE external_games SET store_link = ?, updated_at = now() WHERE id = ?", true},
		{"storefront delete", "DELETE FROM external_games WHERE user_id = ? AND storefront = ?", true},
		{"delete all", "DELETE FROM external_games", true},
		{"lower case", "delete from external_games where id = ?", true},
		{"newline and alias", "\n\t\tUPDATE external_games e\n\t\tSET is_skipped = true", true},
		{"insert", "INSERT INTO external_games (id, user_id) VALUES (?, ?)", true},
		{"platform insert", "INSERT INTO external_game_platforms (id, external_game_id) VALUES (?, ?)", true},
		{"platform delete", "DELETE FROM external_game_platforms WHERE external_game_id = ?", true},
		{"schema and quotes", `UPDATE public."external_games" SET title = ?`, true},
		{"quoted schema", `DELETE FROM "public"."external_game_platforms"`, true},
		{"only", "UPDATE ONLY external_games SET title = ?", true},
		{"truncate", "TRUNCATE external_games", true},
		{"truncate table list", "TRUNCATE TABLE jobs, external_game_platforms CASCADE", true},
		{"merge", "MERGE INTO external_games eg USING src ON eg.id = src.id", true},

		{"changes insert", "INSERT INTO changes (id, job_id, external_game_id, kind) VALUES (?, ?, ?, ?)", false},
		{"user game platforms delete", "DELETE FROM user_game_platforms WHERE external_game_id = ?", false},
		{"locking read", "SELECT id FROM external_games WHERE user_id = ? FOR UPDATE", false},
		{"update jobs reading external games", "UPDATE jobs SET status = 'x' WHERE id IN (SELECT job_id FROM external_games eg WHERE eg.id = ?)", false},
		{"upsert on another table", "INSERT INTO user_games (id) VALUES (?) ON CONFLICT (id) DO UPDATE SET external_games_count = 1", false},
		{"longer table name", "UPDATE external_games_archive SET x = 1", false},
		{"plain read", "SELECT * FROM external_games", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sqlWriteTarget.MatchString(tc.sql); got != tc.write {
				t.Errorf("match(%q) = %v, want %v", tc.sql, got, tc.write)
			}
		})
	}
}

// TestWriteGuardSourceScan pins the Go-level part of the guard: literals,
// concatenations, comments and query-builder chains.
func TestWriteGuardSourceScan(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		write bool
	}{
		{"raw literal", "db.NewRaw(`UPDATE external_games SET title = ? WHERE id = ?`, t, id)", true},
		{"concatenated with a call", "db.NewRaw(`UPDATE external_games eg SET resolved_igdb_id = ? WHERE eg.id = ? AND `+Undecided(\"eg\"), a, b)", true},
		{"split across literals", `db.NewRaw("DELETE FROM " + "external_games")`, true},
		{"builder update on model", "db.NewUpdate().Model((*models.ExternalGame)(nil)).Set(\"title = ?\", x).Where(\"id = ?\", id)", true},
		{"builder insert on platforms model", "db.NewInsert().Model(&models.ExternalGamePlatform{})", true},
		{"builder delete on table expr", `db.NewDelete().TableExpr("external_game_platforms AS p").Where("p.id = ?", id)`, true},
		{"builder delete on quoted table", `db.NewDelete().Table("public.external_games").Where("id = ?", id)`, true},
		{"builder truncate", "db.NewTruncateTable().Model((*models.ExternalGame)(nil))", true},

		{"comment only", "// UPDATE external_games SET title = ?\n_ = 1", false},
		{"builder read", `db.NewSelect().TableExpr("external_games AS eg").Where("eg.id = ?", id)`, false},
		{"builder read on model", "db.NewSelect().Model((*models.ExternalGame)(nil))", false},
		{"usergame unlink", `db.NewDelete().TableExpr("user_game_platforms").Where("external_game_id = ?", id)`, false},
		{"changes insert", "db.NewRaw(`INSERT INTO changes (id, external_game_id) VALUES (?, ?)`, a, b)", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\nfunc f() {\n" + tc.body + "\n}\n"
			got := writesInFile(t, token.NewFileSet(), "x.go", src)
			if (len(got) > 0) != tc.write {
				t.Errorf("writes = %v, want write=%v", got, tc.write)
			}
		})
	}
}

// TestDoesNotLinkTheJobProcess is the import guard: this package sits below
// the Job process and the HTTP layer, so it links neither.
func TestDoesNotLinkTheJobProcess(t *testing.T) {
	out, err := exec.Command(goTool(t), "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	forbidden := []string{
		modulePath + "/internal/joblifecycle",
		modulePath + "/internal/worker/tasks",
		modulePath + "/internal/api",
	}
	for line := range strings.Lines(string(out)) {
		dep := strings.TrimSpace(line)
		for _, f := range forbidden {
			if dep == f || strings.HasPrefix(dep, f+"/") {
				t.Errorf("internal/externalgame links %s", dep)
			}
		}
	}
}
