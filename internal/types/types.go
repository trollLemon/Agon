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

package types

import "time"

// Session is the full on-disk shape of one debate: metadata, transcript, and verdict.
type Session struct {
	SessionID       string            `json:"session_id"`
	Title           string            `json:"title"`
	Topic           string            `json:"topic"`
	StartingContext string            `json:"starting_context,omitempty"`
	Mode            string            `json:"mode"`
	Tone            string            `json:"tone"`
	Rounds          int               `json:"rounds"`
	Sides           []Side            `json:"sides"`
	Model           string            `json:"model"`
	CreatedAt       time.Time         `json:"created_at"`
	Messages        []Message         `json:"messages"`
	Verdict         string            `json:"verdict,omitempty"`
	Aborted         map[string]string `json:"aborted,omitempty"`
	Dirs            []string          `json:"dirs,omitempty"`
	Files           []string          `json:"files,omitempty"`
}

// Side is one debater in a session.
type Side struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Stance string `json:"stance"`
}

// Message is a single transcript entry.
type Message struct {
	Role      string     `json:"role"`
	Round     int        `json:"round"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	TS        float64    `json:"ts"`
}

// ToolCall records one tool invocation made while producing a message.
type ToolCall struct {
	Name          string `json:"name"`
	Args          string `json:"args"`
	ResultSummary string `json:"result_summary"`
}
