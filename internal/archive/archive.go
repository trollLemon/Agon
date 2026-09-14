// Copyright (C) 2026  trolllemon
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package archive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/trollLemon/agon/internal/types"
)

// path returns the archive file path for a session id within dir.
func path(dir, sessionID string) string {
	return filepath.Join(dir, sessionID+".json")
}

// Write persists s atomically to the archive directory. It writes the
// session JSON to a temp file and renames it into place so a reader never
// observes a partial file. Used for fixtures and direct archive creation.
func Write(dir string, s *types.Session) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	tmp, err := os.CreateTemp(dir, s.SessionID+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path(dir, s.SessionID)); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}

// Load reads and parses the one archived session from archive dir.
func Load(dir, sessionID string) (*types.Session, error) {
	b, err := os.ReadFile(path(dir, sessionID))
	if err != nil {
		return nil, err
	}
	var s types.Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse session %q: %w", sessionID, err)
	}
	return &s, nil
}

// List returns every archived session in dir, newest first. A missing dir
// is treated as empty, not an error.
func List(dir string) ([]*types.Session, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read archive dir: %w", err)
	}

	out := make([]*types.Session, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		sessionID := strings.TrimSuffix(name, ".json")
		s, err := Load(dir, sessionID)
		if err != nil {
			continue // skip unreadable/partial files rather than failing the whole listing
		}
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].SessionID > out[j].SessionID
	})
	return out, nil
}
