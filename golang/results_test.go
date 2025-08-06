package golang

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// helper returns a simple test fixture used by several tests.
func sampleResults(tmpDir string) Results {
	file := File{
		Package:            "github.com/foo/bar",
		File:               "foo_test.go",
		FileAbs:            filepath.Join(tmpDir, "foo_test.go"),
		Tests:              []Test{{Name: "TestFoo", OriginalLine: 42}},
		ModifiedSourceCode: "// modified source",
	}

	pkg := PackageResults{
		Package:   "github.com/foo/bar",
		Successes: []File{file},
		Failures:  []string{"TestBroken"},
	}

	return Results{pkg.Package: pkg}
}

// ----------------------------------------------------------------------------
// pastTense helper
// ----------------------------------------------------------------------------

func TestPastTense(t *testing.T) {
	t.Parallel()
	cases := []struct {
		op   OperationType
		want string
	}{
		{OperationQuarantine, "quarantined"},
		{OperationUnquarantine, "unquarantined"},
		{OperationType("custom"), "custom"},
	}

	for _, tc := range cases {
		if got := pastTense(tc.op); got != tc.want {
			t.Fatalf("pastTense(%q) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

// ----------------------------------------------------------------------------
// ResultsView.String / Markdown
// ----------------------------------------------------------------------------

func TestResultsViewStringAndMarkdown(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	results := sampleResults(tmp)

	view := NewResultsView(results, OperationQuarantine)
	str := view.String()

	// Order of map iteration is undefined; assert via key substrings.
	wantSubs := []string{
		"github.com/foo/bar",
		"Successes",
		"foo_test.go",
		"TestFoo",
		"Failures",
		"TestBroken",
	}
	for _, sub := range wantSubs {
		if !strings.Contains(str, sub) {
			t.Errorf("String() output missing %q\noutput:\n%s", sub, str)
		}
	}

	md := view.Markdown("smartcontractkit", "repo", "main")
	if !strings.HasPrefix(md, "# Quarantined Flaky Tests") {
		t.Errorf("Markdown() did not include quarantine heading; got\n%s", md)
	}
	if !strings.Contains(md, "[foo_test.go]") {
		t.Errorf("Markdown() table row missing; got\n%s", md)
	}
}

// ----------------------------------------------------------------------------
// GetCommitInfo
// ----------------------------------------------------------------------------

func TestGetCommitInfo(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	results := sampleResults(tmp)

	msg, updates := GetCommitInfo(results, OperationQuarantine)

	if !strings.HasPrefix(msg, "branch-out quarantine tests") {
		t.Errorf("commit message prefix wrong:\n%s", msg)
	}
	if !strings.Contains(msg, "foo_test.go: TestFoo") {
		t.Errorf("commit message missing file line:\n%s", msg)
	}

	wantUpdates := map[string]string{
		"foo_test.go": "// modified source",
	}
	if !reflect.DeepEqual(updates, wantUpdates) {
		t.Errorf("updates map mismatch:\n got  %#v\n want %#v", updates, wantUpdates)
	}
}

// ----------------------------------------------------------------------------
// writeResultsToFiles
// ----------------------------------------------------------------------------

func TestWriteResultsToFiles(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	results := sampleResults(tmp)

	// silence logger output
	logger := zerolog.New(io.Discard)

	if err := writeResultsToFiles(logger, results, "quarantine"); err != nil {
		t.Fatalf("writeResultsToFiles returned error: %v", err)
	}

	// verify the file was written with correct contents
	outPath := filepath.Join(tmp, "foo_test.go")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if got, want := string(data), "// modified source"; got != want {
		t.Fatalf("file contents = %q, want %q", got, want)
	}
}

// ----------------------------------------------------------------------------
// Results.ForEach consistency (order-independent)
// ----------------------------------------------------------------------------

func TestResultsForEach(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	results := sampleResults(tmp)

	var pkgs []string
	results.ForEach(func(pr PackageResults) { pkgs = append(pkgs, pr.Package) })

	sort.Strings(pkgs)
	if len(pkgs) != 1 || pkgs[0] != "github.com/foo/bar" {
		t.Fatalf("ForEach collected %v, want [github.com/foo/bar]", pkgs)
	}
}
