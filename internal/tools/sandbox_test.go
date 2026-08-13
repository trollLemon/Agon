package tools

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectPaths(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("directory found", func(t *testing.T) {
		text := "please refactor the auth layer in " + dir + " and add tests"
		got, err := DetectPaths(text)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != dir {
			t.Errorf("got %v, want [%s]", got, dir)
		}
	})

	t.Run("duplicates collapsed", func(t *testing.T) {
		text := "look at " + dir + " and also " + dir
		got, err := DetectPaths(text)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("got %v, want one entry", got)
		}
	})

	t.Run("garbage text finds nothing", func(t *testing.T) {
		got, _ := DetectPaths("should we adopt event sourcing for the orders service?")
		if len(got) != 0 {
			t.Errorf("expected no directories, got %v", got)
		}
	})

	t.Run("nonexistent path finds nothing", func(t *testing.T) {
		got, _ := DetectPaths("look at /this/path/does/not/exist-xyz for context")
		if len(got) != 0 {
			t.Errorf("expected no directories, got %v", got)
		}
	})

	t.Run("url is not mistaken for a path", func(t *testing.T) {
		got, _ := DetectPaths("see https://example.com/some/path for docs")
		if len(got) != 0 {
			t.Errorf("expected URLs to be ignored, got %v", got)
		}
	})
}

func TestNewSandboxValidatesDirs(t *testing.T) {
	if _, err := NewSandbox(nil); err == nil {
		t.Error("expected error for no directories")
	}
	if _, err := NewSandbox([]string{"/this/path/does/not/exist-xyz"}); err == nil {
		t.Error("expected error for nonexistent directory")
	}
	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSandbox([]string{t.TempDir(), f}); err == nil {
		t.Error("expected error naming the non-directory entry")
	}
}

func TestSandboxEscapeRejected(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "in.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	sb, err := NewSandbox([]string{root})
	if err != nil {
		t.Fatal(err)
	}

	escapeAttempts := []string{
		"../" + filepath.Base(outsideDir) + "/secret.txt",
		"../../etc/passwd",
		"/etc/passwd",
		filepath.Join(outsideDir, "secret.txt"),
		"..",
	}
	for _, attempt := range escapeAttempts {
		if _, err := sb.ReadFile(attempt); err == nil {
			t.Errorf("ReadFile(%q): expected escape to be rejected", attempt)
		}
	}

	content, err := sb.ReadFile("in.txt")
	if err != nil {
		t.Fatalf("ReadFile(in.txt): %v", err)
	}
	if content != "inside" {
		t.Errorf("got %q, want %q", content, "inside")
	}
}

func TestSandboxSentinelErrors(t *testing.T) {
	if _, err := NewSandbox(nil); !errors.Is(err, ErrNoDirs) {
		t.Errorf("NewSandbox(nil): got %v, want ErrNoDirs", err)
	}

	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSandbox([]string{f}); !errors.Is(err, ErrNotDirectory) {
		t.Errorf("NewSandbox(file): got %v, want ErrNotDirectory", err)
	}

	root := t.TempDir()
	sb, err := NewSandbox([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sb.ReadFile("../escape"); !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("ReadFile(escape): got %v, want ErrOutsideSandbox", err)
	}
	if _, err := sb.ReadFile("/etc/passwd"); !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("ReadFile(absolute outside): got %v, want ErrOutsideSandbox", err)
	}
	if _, err := sb.ReadFile("missing.txt"); !errors.Is(err, ErrNotFound) {
		t.Errorf("ReadFile(missing): got %v, want ErrNotFound", err)
	}
	if _, err := sb.Grep("(", ""); !errors.Is(err, ErrInvalidPattern) {
		t.Errorf("Grep(bad pattern): got %v, want ErrInvalidPattern", err)
	}

	multi, err := NewSandbox([]string{root, t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := multi.ListDir(""); !errors.Is(err, ErrAmbiguousPath) {
		t.Errorf("ListDir(\"\") with multiple dirs: got %v, want ErrAmbiguousPath", err)
	}
}

func TestSandboxMultipleDirs(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	os.WriteFile(filepath.Join(dirA, "a.txt"), []byte("aaa"), 0o644)
	os.WriteFile(filepath.Join(dirB, "b.txt"), []byte("bbb"), 0o644)
	// Same name in both: the more specific (longer) dir wins.
	deep := filepath.Join(dirA, "deep")
	os.Mkdir(deep, 0o755)
	os.WriteFile(filepath.Join(deep, "a.txt"), []byte("deep"), 0o644)

	sb, err := NewSandbox([]string{dirA, deep, dirB})
	if err != nil {
		t.Fatal(err)
	}

	if got, err := sb.ReadFile("b.txt"); err != nil || got != "bbb" {
		t.Errorf("ReadFile(b.txt): got (%q, %v)", got, err)
	}
	if got, err := sb.ReadFile("a.txt"); err != nil || got != "deep" {
		t.Errorf("ReadFile(a.txt): got (%q, %v), want most-specific match", got, err)
	}

	// Absolute paths inside any sandbox dir work.
	if got, err := sb.ReadFile(filepath.Join(dirB, "b.txt")); err != nil || got != "bbb" {
		t.Errorf("ReadFile(absolute): got (%q, %v)", got, err)
	}
	if _, err := sb.ReadFile(filepath.Join(t.TempDir(), "x")); err == nil {
		t.Error("ReadFile: expected rejection for absolute path outside all dirs")
	}

	// Bare "." is ambiguous with more than one dir.
	if _, err := sb.ListDir(""); err == nil {
		t.Error("ListDir(\"\"): expected ambiguity error with multiple dirs")
	}

	entries, err := sb.ListDir(dirB)
	if err != nil || len(entries) != 1 || entries[0] != "b.txt" {
		t.Errorf("ListDir(dirB): got (%v, %v)", entries, err)
	}

	matches, err := sb.Grep("bbb", dirB)
	if err != nil || len(matches) != 1 || matches[0].Path != "b.txt" {
		t.Errorf("Grep: got (%+v, %v)", matches, err)
	}
}

func TestSandboxListDir(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644)
	os.Mkdir(filepath.Join(root, ".git"), 0o755)

	sb, err := NewSandbox([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := sb.ListDir("")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.txt", "sub/"}
	if len(entries) != len(want) {
		t.Fatalf("got %v, want %v", entries, want)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, entries[i], want[i])
		}
	}
}

func TestSandboxGrep(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "other.go"), []byte("package other\n"), 0o644)

	sb, err := NewSandbox([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := sb.Grep(`func \w+\(`, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Path != "main.go" || matches[0].Line != 2 {
		t.Errorf("got %+v, want one match at main.go:2", matches)
	}
}

func TestCallDispatch(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "f.txt"), []byte("hello"), 0o644)
	sb, err := NewSandbox([]string{root})
	if err != nil {
		t.Fatal(err)
	}

	out, err := Call(sb, "read_file", map[string]any{"path": "f.txt"})
	if err != nil || out != "hello" {
		t.Errorf("read_file: got (%q, %v)", out, err)
	}

	if _, err := Call(sb, "read_file", map[string]any{"path": "../escape"}); err == nil {
		t.Errorf("expected escape via Call to be rejected")
	}

	if _, err := Call(sb, "nonexistent_tool", nil); err == nil {
		t.Errorf("expected unknown tool to error")
	}
}
