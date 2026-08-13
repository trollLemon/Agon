// Package tools implements the read-only, sandboxed tool set debaters use to
// ground claims in real directories: read_file, grep, and list_dir. All
// three are confined to a set of detected directories; nothing in this
// package can write, delete, or read outside them. The directories are
// plain paths — they may or may not be git repositories.
package tools

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// ignoredDirs are skipped while walking a sandbox tree.
var ignoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".venv":        true,
	".idea":        true,
	".vscode":      true,
}

// maxGrepMatches bounds how many matches Grep returns, so a broad pattern
// against a large tree can't blow up a turn.
const maxGrepMatches = 200

var (
	// ErrNoDirs is returned by NewSandbox when no directories are given.
	ErrNoDirs = errors.New("sandbox needs at least one directory")

	// ErrNotDirectory is returned when a sandbox entry is not a directory.
	ErrNotDirectory = errors.New("not a directory")

	// ErrAmbiguousPath is returned when a bare "." cannot be resolved because
	// the sandbox has more than one directory.
	ErrAmbiguousPath = errors.New("path is ambiguous across sandbox directories; use an absolute path")

	// ErrOutsideSandbox is returned when a path resolves outside every sandbox
	// directory, whether an escaping relative path or an unrelated absolute one.
	ErrOutsideSandbox = errors.New("path is outside the sandbox directories")

	// ErrNotFound is returned when a path exists in none of the sandbox directories.
	ErrNotFound = errors.New("path not found in any sandbox directory")

	// ErrInvalidPattern is returned by Grep when the pattern is not a valid regexp.
	ErrInvalidPattern = errors.New("invalid grep pattern")

	// errStopWalk unwinds filepath.WalkDir once maxGrepMatches is reached.
	errStopWalk = errors.New("stop walking")
)

// pathTokenRe finds path-shaped tokens in freeform text: a run of characters
// containing at least one '/', or starting with './', '../', or '~/'.
var pathTokenRe = regexp.MustCompile(`(?:/|\.\.?/|~/)?[\w.@+-]+(?:/[\w.@+-]+)+/?|\.\.?/[\w.@+-]+/?`)

// DetectPaths scans freeform text for path segments that resolve to existing
// directories, returning their absolute forms. It returns nil if no such path is found.
func DetectPaths(text string) ([]string, error) {
	seen := map[string]bool{}
	var dirs []string
	for _, tok := range pathTokenRe.FindAllString(text, -1) {
		tok = strings.Trim(tok, `"'`+"`"+`,;:()[]{}<>`)
		if tok == "" || strings.Contains(tok, "://") {
			continue
		}
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(tok, "~/") {
			tok = filepath.Join(home, strings.TrimPrefix(tok, "~/"))
		}
		abs, err := filepath.Abs(tok)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() && !seen[abs] {
			seen[abs] = true
			dirs = append(dirs, abs)
		}
	}
	return dirs, nil
}

// Sandbox confines read-only file operations to a set of directories.
type Sandbox struct {
	dirs []string // absolute, most specific (longest) first
}

// NewSandbox creates a Sandbox over dirs, each of which must already exist
// and be a directory; the first offending entry fails the call.
func NewSandbox(dirs []string) (*Sandbox, error) {
	if len(dirs) == 0 {
		return nil, ErrNoDirs
	}
	abs := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		a, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("resolve sandbox dir %q: %w", dir, err)
		}
		info, err := os.Stat(a)
		if err != nil {
			return nil, fmt.Errorf("sandbox dir %q: %w", dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("sandbox dir %q: %w", a, ErrNotDirectory)
		}
		if !slices.Contains(abs, a) {
			abs = append(abs, a)
		}
	}
	sort.Slice(abs, func(i, j int) bool { return len(abs[i]) > len(abs[j]) })
	return &Sandbox{dirs: abs}, nil
}

// Dirs returns the sandbox's absolute directories, most specific first.
func (s *Sandbox) Dirs() []string { return slices.Clone(s.dirs) }

// resolve maps a caller-supplied path to an absolute path inside the
// sandbox. An absolute path is accepted only if it stays within one of the
// sandbox dirs (checked lexically via filepath.Rel + filepath.IsLocal).
// Relative paths must be local (no escape) and are joined onto each sandbox
// dir, most specific first, resolving to the first that exists.
// A bare "." is only meaningful with a single directory.
func (s *Sandbox) resolve(p string) (string, error) {
	if p == "" {
		p = "."
	}
	if p == "." {
		if len(s.dirs) == 1 {
			return s.dirs[0], nil
		}
		return "", fmt.Errorf("path %q: %w", p, ErrAmbiguousPath)
	}

	if filepath.IsAbs(p) {
		full := filepath.Clean(p)
		for _, dir := range s.dirs {
			if rel, err := filepath.Rel(dir, full); err == nil && filepath.IsLocal(rel) {
				return full, nil
			}
		}
		return "", fmt.Errorf("path %q: %w", p, ErrOutsideSandbox)
	}

	if !filepath.IsLocal(p) {
		return "", fmt.Errorf("path %q: %w", p, ErrOutsideSandbox)
	}
	for _, dir := range s.dirs {
		if full := filepath.Join(dir, p); statExists(full) {
			return full, nil
		}
	}
	return "", fmt.Errorf("path %q: %w", p, ErrNotFound)
}

func statExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ReadFile returns the contents of a file at p, which may be absolute
// (inside the sandbox) or relative to a sandbox directory.
func (s *Sandbox) ReadFile(p string) (string, error) {
	full, err := s.resolve(p)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ListDir lists the entries of the directory at p, directories suffixed
// with "/".
func (s *Sandbox) ListDir(p string) ([]string, error) {
	full, err := s.resolve(p)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if ignoredDirs[e.Name()] {
			continue
		}
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// Match is a single grep hit.
type Match struct {
	Path string
	Line int
	Text string
}

// relToSandbox returns path relative to its containing sandbox directory.
func (s *Sandbox) relToSandbox(path string) string {
	for _, dir := range s.dirs {
		if rel, err := filepath.Rel(dir, path); err == nil && filepath.IsLocal(rel) {
			return rel
		}
	}
	return path
}

// Grep searches files under p for lines matching pattern (a Go regexp),
// returning up to maxGrepMatches hits in deterministic path/line order.
func (s *Sandbox) Grep(pattern, p string) ([]Match, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPattern, err)
	}
	root, err := s.resolve(p)
	if err != nil {
		return nil, err
	}

	var matches []Match
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if len(matches) >= maxGrepMatches {
			return errStopWalk
		}

		if d.IsDir() {
			if ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		relPath := s.relToSandbox(path)
		lineNo := 0
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if re.MatchString(line) {
				matches = append(matches,
					Match{
						Path: relPath,
						Line: lineNo,
						Text: line})

				if len(matches) >= maxGrepMatches {
					return errStopWalk
				}
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStopWalk) {
		return nil, err
	}
	return matches, nil
}
