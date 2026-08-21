package tools

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePathList(t *testing.T) {
	home, _ := os.UserHomeDir()
	text := "  /a/b  \n\n\"/c/d\"\n/a/b\n~/notes.txt\n"
	got := ParsePathList(text)
	want := []string{"/a/b", "/c/d", filepath.Join(home, "notes.txt")}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, got[i], want[i])
		}
	}
	if len(ParsePathList("   \n\t\n")) != 0 {
		t.Error("expected blank input to yield no paths")
	}
}

func TestNewSandboxClassifiesFilesAndDirs(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notes.md")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	sb, err := NewSandbox([]string{root, file})
	if err != nil {
		t.Fatal(err)
	}
	if got := sb.Dirs(); len(got) != 1 || got[0] != root {
		t.Errorf("Dirs: got %v, want [%s]", got, root)
	}
	if got := sb.Files(); len(got) != 1 || got[0] != file {
		t.Errorf("Files: got %v, want [%s]", got, file)
	}

	if _, err := NewSandbox(nil); !errors.Is(err, ErrNoDirs) {
		t.Errorf("NewSandbox(nil): got %v, want ErrNoDirs", err)
	}
	if _, err := NewSandbox([]string{filepath.Join(root, "missing")}); err == nil {
		t.Error("expected error for a nonexistent path")
	}
}

func TestFileSandboxToolAccess(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed.go")
	other := filepath.Join(root, "other.go")
	if err := os.WriteFile(allowed, []byte("package a\nfunc Do() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("package a\nfunc Secret() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sb, err := NewSandbox([]string{allowed})
	if err != nil {
		t.Fatal(err)
	}

	if got, err := sb.ReadFile(allowed); err != nil || got != "package a\nfunc Do() {}\n" {
		t.Errorf("ReadFile(allowed): got (%q, %v)", got, err)
	}
	if got, err := sb.ReadFile("allowed.go"); err != nil || got != "package a\nfunc Do() {}\n" {
		t.Errorf("ReadFile(basename): got (%q, %v)", got, err)
	}
	if _, err := sb.ReadFile(other); !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("ReadFile(other): got %v, want ErrOutsideSandbox", err)
	}

	matches, err := sb.Grep(`func \w+\(`, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Line != 2 {
		t.Errorf("Grep across files: got %+v, want one hit on line 2 of the allowed file", matches)
	}

	entries, err := sb.ListDir("")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0] != allowed {
		t.Errorf("ListDir default: got %v, want [%s]", entries, allowed)
	}
}

func TestNewSandboxValidatesPaths(t *testing.T) {
	if _, err := NewSandbox(nil); err == nil {
		t.Error("expected error for no directories")
	}
	if _, err := NewSandbox([]string{"/this/path/does/not/exist-xyz"}); err == nil {
		t.Error("expected error for nonexistent directory")
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

func TestSpecsIncludesHTTPGet(t *testing.T) {
	specs := Specs()
	var found bool
	for _, s := range specs {
		if s.Name == "http_get" {
			found = true
			if s.Description == "" {
				t.Error("http_get spec missing description")
			}
			props, ok := s.Parameters["properties"].(map[string]any)
			if !ok {
				t.Fatal("http_get spec missing properties map")
			}
			if _, ok := props["link"]; !ok {
				t.Error("http_get properties missing link parameter")
			}
			req, ok := s.Parameters["required"].([]string)
			if !ok || len(req) != 1 || req[0] != "link" {
				t.Errorf("http_get required: got %v, want [link]", req)
			}
		}
	}
	if !found {
		t.Error("Specs() did not return http_get tool")
	}
}

func TestSandboxLinks(t *testing.T) {
	url1 := "https://example.com/page"
	url2 := "http://example.org/api"

	sb, err := NewSandbox([]string{url1, url2})
	if err != nil {
		t.Fatal(err)
	}

	links := sb.Links()
	if len(links) != 2 || links[0] != url1 || links[1] != url2 {
		t.Errorf("Links: got %v, want [%s, %s]", links, url1, url2)
	}

	resolved, err := sb.resolve(url1)
	if err != nil || resolved != url1 {
		t.Errorf("resolve(url1): got (%q, %v), want (%q, nil)", resolved, err, url1)
	}

	if _, err := sb.resolve("https://unallowed.com"); !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("resolve(unallowed): got %v, want ErrOutsideSandbox", err)
	}
}

func TestURLRegexForms(t *testing.T) {
	for _, u := range []string{
		"https://example.com/page",
		"http://example.org/api?q=1",
		"https://example.com#section",
		"HTTPS://example.com/up",
		"http://127.0.0.1:8080/x",
	} {
		sb, err := NewSandbox([]string{u})
		if err != nil {
			t.Fatalf("NewSandbox(%q): %v", u, err)
		}
		if resolved, err := sb.resolve(u); err != nil || resolved != u {
			t.Errorf("resolve(%q): got (%q, %v), want exact match", u, resolved, err)
		}
	}

	for _, u := range []string{"ftp://example.com", "https://", "/etc/passwd"} {
		if urlRegex.MatchString(u) {
			t.Errorf("urlRegex unexpectedly matched %q", u)
		}
	}
}

func TestHTTPGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/notfound" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("webpage body content"))
	}))
	defer ts.Close()

	sb, err := NewSandbox([]string{ts.URL, ts.URL + "/notfound"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := sb.HTTPGet(ts.URL)
	if err != nil {
		t.Fatalf("HTTPGet: %v", err)
	}
	if got != "webpage body content" {
		t.Errorf("got %q, want %q", got, "webpage body content")
	}

	if _, err := sb.HTTPGet(ts.URL + "/notfound"); err == nil {
		t.Error("expected error for non-200 HTTP response")
	}

	if _, err := sb.HTTPGet("http://unauthorized.org"); !errors.Is(err, ErrOutsideSandbox) {
		t.Errorf("expected ErrOutsideSandbox for unauthorized URL, got %v", err)
	}
}

func TestHTTPGetRedirectsOnce(t *testing.T) {
	mux := http.NewServeMux()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("after one hop"))
	})
	mux.HandleFunc("/hop1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ts.URL+"/hop2", http.StatusFound)
	})
	mux.HandleFunc("/hop2", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, ts.URL+"/final", http.StatusFound)
	})

	sb, err := NewSandbox([]string{ts.URL + "/hop1"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := sb.HTTPGet(ts.URL + "/hop1"); err == nil {
		t.Error("expected error when a second redirect is refused")
	}

	sb2, err := NewSandbox([]string{ts.URL + "/final"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := sb2.HTTPGet(ts.URL + "/final")
	if err != nil {
		t.Fatalf("HTTPGet(/final): %v", err)
	}
	if got != "after one hop" {
		t.Errorf("got %q, want %q", got, "after one hop")
	}
}

func TestCallHTTPGet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("tool call response"))
	}))
	defer ts.Close()

	sb, err := NewSandbox([]string{ts.URL})
	if err != nil {
		t.Fatal(err)
	}

	out, err := Call(sb, "http_get", map[string]any{"link": ts.URL})
	if err != nil {
		t.Fatalf("Call(http_get): %v", err)
	}
	if out != "tool call response" {
		t.Errorf("got %q, want %q", out, "tool call response")
	}

	if _, err := Call(sb, "http_get", map[string]any{}); err == nil {
		t.Error("expected missing link to return error")
	}

	if _, err := Call(sb, "http_get", map[string]any{"link": "http://other.org"}); err == nil {
		t.Error("expected unauthorized link to return error")
	}
}
