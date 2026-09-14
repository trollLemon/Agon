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

package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/trollLemon/agon/internal/types"
)

// SessionCacheFileName is the name of the SESSION_CACHE file stored in a
// session's cache directory.
const SessionCacheFileName = "SESSION_CACHE"

// InProgressMarker is the fixed name for the in-progress indicator file.
const InProgressMarker = "IN_PROGRESS"

// InitCache initializes a new cache directory structure for a session identified
// by a UUID. It creates the directory under root, writes the session JSON to
// SESSION_CACHE, and creates an IN_PROGRESS marker file.
//
// Returns the path to the session directory (root/uuid).
func InitCache(rootDir string, id string, s *types.Session) error {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create cache root dir: %w", err)
	}

	dirPath := filepath.Join(rootDir, id)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	b, err := json.MarshalIndent(*s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	cachePath := filepath.Join(dirPath, SessionCacheFileName)
	tmp, err := os.CreateTemp(dirPath, ".*.tmp")
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

	if err := os.Rename(tmpPath, cachePath); err != nil {
		return fmt.Errorf("rename cache into place: %w", err)
	}

	ipPath := filepath.Join(dirPath, InProgressMarker)
	if err := os.WriteFile(ipPath, []byte(""), 0o644); err != nil {
		return fmt.Errorf("create in-progress marker: %w", err)
	}

	return nil
}

// UpdateCache atomically replaces the SESSION_CACHE file for a session.
// The session directory is expected at root/id/.
func UpdateCache(rootDir string, id string, s *types.Session) error {
	dirPath := filepath.Join(rootDir, id)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	cachePath := filepath.Join(dirPath, SessionCacheFileName)

	b, err := json.MarshalIndent(*s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	tmp, err := os.CreateTemp(dirPath, ".*.tmp")
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

	if err := os.Rename(tmpPath, cachePath); err != nil {
		return fmt.Errorf("rename cache into place: %w", err)
	}

	return nil
}

// WasInterrupted checks whether a session was in progress before the app was
// closed by looking for the IN_PROGRESS marker file.
func WasInterrupted(rootDir string, id string) (bool, error) {
	ipPath := filepath.Join(rootDir, id, InProgressMarker)
	_, err := os.Stat(ipPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Load reads and parses one cached session from root/sessionID/SESSION_CACHE.
func Load(rootDir, sessionID string) (*types.Session, error) {
	cachePath := filepath.Join(rootDir, sessionID, SessionCacheFileName)
	b, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	var s types.Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse session %q: %w", sessionID, err)
	}
	return &s, nil
}

// List returns every cached session in root, newest first. A missing root
// is treated as empty, not an error.
func List(rootDir string) ([]*types.Session, error) {
	entries, err := os.ReadDir(rootDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache root dir: %w", err)
	}

	out := make([]*types.Session, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() {
			continue
		}
		s, err := Load(rootDir, name)
		if err != nil {
			continue // skip unreadable/partial dirs
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

// ListInterrupted returns only sessions that still have the IN_PROGRESS
// sentinel, i.e. debates interrupted before archiving.
func ListInterrupted(rootDir string) ([]*types.Session, error) {
	all, err := List(rootDir)
	if err != nil {
		return nil, err
	}
	out := make([]*types.Session, 0, len(all))
	for _, s := range all {
		ok, err := WasInterrupted(rootDir, s.SessionID)
		if err != nil {
			continue
		}
		if ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// Remove deletes the cache directory for a session, including its
// IN_PROGRESS marker and SESSION_CACHE file.
func Remove(rootDir, sessionID string) error {
	dirPath := filepath.Join(rootDir, sessionID)
	if err := os.RemoveAll(dirPath); err != nil {
		return fmt.Errorf("remove cache dir %q: %w", dirPath, err)
	}
	return nil
}
