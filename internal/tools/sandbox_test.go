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
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "parses paths with quotes and duplicates",
			input: "  /a/b  \n\n\"/c/d\"\n/a/b\n~/notes.txt\n",
			want:  []string{"/a/b", "/c/d", filepath.Join(home, "notes.txt")},
		},
		{
			name:  "blank input yields empty",
			input: "   \n\t\n",
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParsePathList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNewSandboxClassifiesFilesAndDirs(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notes.md")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		paths     []string
		wantDirs  []string
		wantFiles []string
		wantErr   error
	}{
		{
			name:      "classifies dirs and files",
			paths:     []string{root, file},
			wantDirs:  []string{root},
			wantFiles: []string{file},
		},
		{
			name:    "nil paths returns ErrNoDirs",
			paths:   nil,
			wantErr: ErrNoDirs,
		},
		{
			name:    "nonexistent path returns error",
			paths:   []string{filepath.Join(root, "missing")},
			wantErr: os.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb, err := NewSandbox(tt.paths)
			if tt.wantErr != nil {
				var wantErr error = tt.wantErr
				if !errors.Is(err, wantErr) {
					t.Errorf("expected %v, got %v", wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := sb.Dirs(); len(got) != len(tt.wantDirs) || (len(got) > 0 && got[0] != tt.wantDirs[0]) {
				t.Errorf("Dirs: got %v, want %v", got, tt.wantDirs)
			}
			if got := sb.Files(); len(got) != len(tt.wantFiles) || (len(got) > 0 && got[0] != tt.wantFiles[0]) {
				t.Errorf("Files: got %v, want %v", got, tt.wantFiles)
			}
		})
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

	tests := []struct {
		name    string
		fn      func() (string, error)
		want    string
		wantErr error
	}{
		{
			name: "ReadFile with full path",
			fn:   func() (string, error) { return sb.ReadFile(allowed) },
			want: "package a\nfunc Do() {}\n",
		},
		{
			name: "ReadFile with basename",
			fn:   func() (string, error) { return sb.ReadFile("allowed.go") },
			want: "package a\nfunc Do() {}\n",
		},
		{
			name:    "ReadFile outside sandbox returns ErrOutsideSandbox",
			fn:      func() (string, error) { return sb.ReadFile(other) },
			wantErr: ErrOutsideSandbox,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
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
	tests := []struct {
		name    string
		paths   []string
		wantErr error
	}{
		{"nil paths", nil, ErrNoDirs},
		{"nonexistent directory", []string{"/this/path/does/not/exist-xyz"}, os.ErrNotExist},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSandbox(tt.paths)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
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
		t.Run(attempt, func(t *testing.T) {
			if _, err := sb.ReadFile(attempt); err == nil {
				t.Errorf("ReadFile(%q): expected escape to be rejected", attempt)
			}
		})
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
	tests := []struct {
		name    string
		setup   func() *Sandbox
		op      func(*Sandbox) error
		wantErr error
	}{
		{
			name:  "nil paths returns ErrNoDirs",
			setup: func() *Sandbox { return nil },
			op: func(s *Sandbox) error {
				_, err := NewSandbox(nil)
				return err
			},
			wantErr: ErrNoDirs,
		},
		{
			name: "escape returns ErrOutsideSandbox",
			setup: func() *Sandbox {
				root := t.TempDir()
				sb, _ := NewSandbox([]string{root})
				return sb
			},
			op: func(s *Sandbox) error {
				_, err := s.ReadFile("../escape")
				return err
			},
			wantErr: ErrOutsideSandbox,
		},
		{
			name: "absolute outside returns ErrOutsideSandbox",
			setup: func() *Sandbox {
				root := t.TempDir()
				sb, _ := NewSandbox([]string{root})
				return sb
			},
			op: func(s *Sandbox) error {
				_, err := s.ReadFile("/etc/passwd")
				return err
			},
			wantErr: ErrOutsideSandbox,
		},
		{
			name: "missing file returns ErrNotFound",
			setup: func() *Sandbox {
				root := t.TempDir()
				sb, _ := NewSandbox([]string{root})
				return sb
			},
			op: func(s *Sandbox) error {
				_, err := s.ReadFile("missing.txt")
				return err
			},
			wantErr: ErrNotFound,
		},
		{
			name: "invalid grep pattern returns ErrInvalidPattern",
			setup: func() *Sandbox {
				root := t.TempDir()
				sb, _ := NewSandbox([]string{root})
				return sb
			},
			op: func(s *Sandbox) error {
				_, err := s.Grep("(", "")
				return err
			},
			wantErr: ErrInvalidPattern,
		},
		{
			name: "multiple dirs ListDir ambiguous returns ErrAmbiguousPath",
			setup: func() *Sandbox {
				root := t.TempDir()
				root2 := t.TempDir()
				sb, _ := NewSandbox([]string{root, root2})
				return sb
			},
			op: func(s *Sandbox) error {
				_, err := s.ListDir("")
				return err
			},
			wantErr: ErrAmbiguousPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sb := tt.setup()
			err := tt.op(sb)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("got %v, want %v", err, tt.wantErr)
			}
		})
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
