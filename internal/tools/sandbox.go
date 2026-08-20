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

// roots returns every top-level sandbox path: directories first (most
// specific first), then explicitly allowed files.
func deriveRoots(dirs []string, files []string) []string {
	roots := make([]string, 0, len(dirs)+len(files))
	roots = append(roots, dirs...)
	roots = append(roots, files...)
	return roots
}

// ParsePathList splits a user-entered, newline-separated list of paths into
// cleaned entries: it trims surrounding whitespace and quotes, expands a
// leading "~/", drops blank lines, and collapses duplicates. Existence and
// file-vs-directory classification are left to NewSandbox.
func ParsePathList(text string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, line := range strings.Split(text, "\n") {
		p := strings.TrimSpace(line)
		p = strings.Trim(p, `"'`+"`")
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, strings.TrimPrefix(p, "~/"))
			}
		}
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// Sandbox confines read-only file operations to a set of directories and/or
// individual files. Directories ground the tools over their whole tree; files
// grant access to exactly that file.
type Sandbox struct {
	dirs  []string // absolute, most specific (longest) first
	files []string // absolute individual files, sorted
	roots []string // roots for every top level path
}

// NewSandbox creates a Sandbox over a mix of existing files and
// directories. Each path must already exist; the first missing entry fails
// the call. Directories ground the tools over their whole tree, files grant
// access to exactly that file.
func NewSandbox(paths []string) (*Sandbox, error) {
	if len(paths) == 0 {
		return nil, ErrNoDirs
	}
	var dirs, files []string
	for _, p := range paths {
		a, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolve sandbox path %q: %w", p, err)
		}
		info, err := os.Stat(a)
		if err != nil {
			return nil, fmt.Errorf("sandbox path %q: %w", p, err)
		}
		if info.IsDir() {
			if !slices.Contains(dirs, a) {
				dirs = append(dirs, a)
			}
			continue
		}
		if !slices.Contains(files, a) {
			files = append(files, a)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	sort.Strings(files)

	roots := deriveRoots(dirs, files)

	return &Sandbox{dirs: dirs, files: files, roots: roots}, nil
}

// Dirs returns the sandbox's absolute directories, most specific first.
func (s *Sandbox) Dirs() []string { return s.dirs }

// Files returns the sandbox's explicitly allowed absolute file paths.
func (s *Sandbox) Files() []string { return s.files }

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
		if len(s.roots) == 1 {
			return s.roots[0], nil
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
		for _, f := range s.files {
			if full == f {
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
	for _, f := range s.files {
		if filepath.Base(f) == p {
			return f, nil
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
// with "/". With a default path ("" or "."), a sandbox that holds explicitly
// allowed files lists its top-level roots instead of a single directory.
func (s *Sandbox) ListDir(p string) ([]string, error) {
	if (p == "" || p == ".") && len(s.files) > 0 {
		return s.rootListing(), nil
	}
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

// rootListing renders the sandbox's top-level roots for a default list_dir:
// directories (suffixed with "/") and explicitly allowed files, each as its
// absolute path, in deterministic order.
func (s *Sandbox) rootListing() []string {
	names := make([]string, 0, len(s.dirs)+len(s.files))
	for _, d := range s.dirs {
		names = append(names, d+"/")
	}
	names = append(names, s.files...)
	sort.Strings(names)
	return names
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
// returning up to maxGrepMatches hits in deterministic path/line order. With
// a default path ("" or "."), every sandbox root is searched: each directory
// tree and each explicitly allowed file.
func (s *Sandbox) Grep(pattern, p string) ([]Match, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPattern, err)
	}

	var roots []string
	if p == "" || p == "." {
		roots = s.roots
	} else {
		root, err := s.resolve(p)
		if err != nil {
			return nil, err
		}
		roots = []string{root}
	}

	var matches []Match
	for _, root := range roots {
		if len(matches) >= maxGrepMatches {
			break
		}
		info, err := os.Stat(root)
		if err != nil {
			continue
		}
		var werr error
		if info.IsDir() {
			werr = s.grepDir(re, root, &matches)
		} else {
			werr = s.grepFile(re, root, &matches)
		}
		if werr != nil && !errors.Is(werr, errStopWalk) {
			return nil, werr
		}
	}
	return matches, nil
}

// grepDir walks a directory tree, appending matches, skipping ignoredDirs and
// unwinding via errStopWalk once maxGrepMatches is reached.
func (s *Sandbox) grepDir(re *regexp.Regexp, root string, matches *[]Match) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if len(*matches) >= maxGrepMatches {
			return errStopWalk
		}
		if d.IsDir() {
			if ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		return s.grepFile(re, path, matches)
	})
}

// grepFile scans one file, appending matching lines. Unreadable files are
// skipped; it returns errStopWalk once maxGrepMatches is reached.
func (s *Sandbox) grepFile(re *regexp.Regexp, path string, matches *[]Match) error {
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
		if len(*matches) >= maxGrepMatches {
			return errStopWalk
		}
		line := scanner.Text()
		if re.MatchString(line) {
			*matches = append(*matches, Match{Path: relPath, Line: lineNo, Text: line})
		}
	}
	return nil
}
