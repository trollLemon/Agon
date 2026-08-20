package tools

import "fmt"

// Spec describes one tool's calling contract in a transport-neutral shape:
// callers (e.g. the kronk chat client adapter) translate this into whatever
// tool-document format the model backend expects.
type Spec struct {
	Name        string
	Description string
	// Parameters is a JSON-schema object document, e.g.
	// {"type":"object","properties":{...},"required":[...]}.
	Parameters map[string]any
}

// Specs returns the tool set exposed to debaters when a repo is in scope:
// read_file, grep, list_dir, and http_get.
func Specs() []Spec {
	return []Spec{
		{
			Name:        "read_file",
			Description: "Read the full contents of a file in the sandboxed directories under debate.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute path within a sandbox directory, or relative to one.",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "grep",
			Description: "Search files in the sandboxed directories under debate for lines matching a regular expression.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Go regular expression to search for.",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Directory to search under: absolute within a sandbox directory, or relative to one. Defaults to the sandbox root when there is exactly one.",
					},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "list_dir",
			Description: "List the entries of a directory in the sandboxed directories under debate.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Directory path: absolute within a sandbox directory, or relative to one. Defaults to the sandbox root when there is exactly one.",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "http_get",
			Description: "Fetch the content of an allowed web page at the specified link.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"link": map[string]any{
						"type":        "string",
						"description": "URL of the webpage to fetch.",
					},
				},
				"required": []string{"link"},
			},
		},
	}
}

// Call dispatches one tool invocation by name against the sandbox, returning
// the result text a model should see next.
func Call(sb *Sandbox, name string, args map[string]any) (string, error) {
	switch name {
	case "read_file":
		return sb.ReadFile(args["path"].(string))

	case "grep":
		pattern := args["pattern"].(string)
		if pattern == "" {
			return "", fmt.Errorf("grep requires a pattern")
		}
		matches, err := sb.Grep(pattern, args["path"].(string))
		if err != nil {
			return "", err
		}
		if len(matches) == 0 {
			return "no matches", nil
		}
		out := ""
		for _, m := range matches {
			out += fmt.Sprintf("%s:%d: %s\n", m.Path, m.Line, m.Text)
		}
		return out, nil

	case "list_dir":
		names, err := sb.ListDir(args["path"].(string))
		if err != nil {
			return "", err
		}
		out := ""
		for _, n := range names {
			out += n + "\n"
		}
		return out, nil

	case "http_get":
		link := args["link"].(string)
		if link == "" {
			return "", fmt.Errorf("http_get requires a link")
		}
		return sb.http_get(link)

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}
