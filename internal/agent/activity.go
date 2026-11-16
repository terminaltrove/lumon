package agent

import (
	"encoding/json"
	"hash/fnv"
	"path"
	"strings"
)

// Activity names what an agent is doing from one tool call: "Editing view.go",
// "Reading main.go", a shell command's own description, and so on. Every agent
// names its tools its own way, so the rule goes by the words in the name. The
// path is the file an edit changes, or "" for anything else. A shell command is
// never shown in full, since it can hold a secret.
func Activity(tool string, input json.RawMessage) (doing, edited string) {
	var in struct {
		Description string          `json:"description"`
		Command     json.RawMessage `json:"command"`

		FilePath     string `json:"file_path"`
		FilePathV    string `json:"filePath"`
		Path         string `json:"path"`
		AbsolutePath string `json:"absolute_path"`
		NotebookPath string `json:"notebook_path"`
	}
	json.Unmarshal(input, &in)
	file := firstOf(in.FilePath, in.FilePathV, in.AbsolutePath, in.NotebookPath, in.Path)

	name := strings.ToLower(tool)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(name, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("todo"):
		return "Planning", ""
	case has("task", "agent"):
		return "Running a subagent", ""
	case has("web", "fetch", "url"):
		return "Reading the web", ""
	case has("bash", "shell", "exec", "command", "terminal"):
		if in.Description != "" {
			return in.Description, ""
		}
		if w := firstWord(in.Command); w != "" {
			return "Running " + w, ""
		}
		return "Running a command", ""
	case has("edit", "write", "replace", "patch", "create"):
		if file == "" {
			return "Editing files", ""
		}
		return "Editing " + base(file), file
	case has("read", "view"):
		if file == "" {
			return "Reading files", ""
		}
		return "Reading " + base(file), ""
	case has("grep", "glob", "search", "find", "list") || name == "ls":
		return "Searching", ""
	case tool == "":
		return "Working", ""
	}
	return tool, ""
}

func firstOf(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// base is a path's last element, whichever separator it uses.
func base(p string) string {
	return path.Base(strings.ReplaceAll(p, `\`, "/"))
}

// firstWord is the program a command runs: a string's first word that is not
// a variable assignment, or for a shell argument list such as
// ["bash", "-lc", "go test ./..."], the script's.
func firstWord(raw json.RawMessage) string {
	var line string
	var args []string
	if json.Unmarshal(raw, &line) != nil {
		if json.Unmarshal(raw, &args) != nil || len(args) == 0 {
			return ""
		}
		line = strings.Join(args, " ")
		if len(args) >= 3 && strings.HasPrefix(args[len(args)-2], "-") {
			line = args[len(args)-1]
		}
	}
	for _, w := range strings.Fields(line) {
		if !strings.Contains(w, "=") {
			return base(w)
		}
	}
	return ""
}

// editHash is how a turn remembers an edited file without keeping its path.
func editHash(p string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(p))
	return h.Sum32()
}
