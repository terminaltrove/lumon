package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Each agent names its tools its own way, and the step reads the same.
func TestEveryAgentsToolsReadAsPlainSteps(t *testing.T) {
	for _, tc := range []struct {
		tool, input string
		doing       string
		edited      string
	}{
		// Claude Code.
		{"Edit", `{"file_path":"/src/lumon/view.go","old_string":"a"}`, "Editing view.go", "/src/lumon/view.go"},
		{"Write", `{"file_path":"/src/new.go","content":"package x"}`, "Editing new.go", "/src/new.go"},
		{"Read", `{"file_path":"/src/main.go"}`, "Reading main.go", ""},
		{"Bash", `{"command":"go test ./...","description":"Run the tests"}`, "Run the tests", ""},
		{"Bash", `{"command":"GOOS=linux go build ./cmd/lumon"}`, "Running go", ""},
		{"Grep", `{"pattern":"TODO"}`, "Searching", ""},
		{"WebFetch", `{"url":"https://example.com"}`, "Reading the web", ""},
		{"Task", `{"prompt":"look around"}`, "Running a subagent", ""},
		{"TodoWrite", `{"todos":[]}`, "Planning", ""},
		// Alternate tool spellings.
		{"replace", `{"file_path":"/src/a.go"}`, "Editing a.go", "/src/a.go"},
		{"run_shell_command", `{"command":"make check","description":"Check everything"}`, "Check everything", ""},
		{"read_many_files", `{"paths":["a","b"]}`, "Reading files", ""},
		{"google_web_search", `{"query":"lumon"}`, "Reading the web", ""},
		{"list_directory", `{"path":"/src"}`, "Searching", ""},
		// Shell argument lists and generic patches.
		{"Bash", `{"command":["bash","-lc","cargo build --release"]}`, "Running cargo", ""},
		{"apply_patch", `{"input":"*** Begin Patch"}`, "Editing files", ""},
		// Pi and OpenCode.
		{"edit", `{"path":"src/b.ts"}`, "Editing b.ts", "src/b.ts"},
		{"edit", `{"filePath":"C:\\src\\c.ts"}`, "Editing c.ts", `C:\src\c.ts`},
		{"bash", `{"command":"bun test"}`, "Running bun", ""},
		{"ls", `{}`, "Searching", ""},
		// Anything else is named as it is.
		{"mcp__notes__save", `{}`, "mcp__notes__save", ""},
	} {
		doing, edited := Activity(tc.tool, json.RawMessage(tc.input))
		if doing != tc.doing || edited != tc.edited {
			t.Errorf("%s %s: got (%q, %q), want (%q, %q)", tc.tool, tc.input, doing, edited, tc.doing, tc.edited)
		}
	}
}

// A command is never shown in full, since it can carry a secret.
func TestAShellCommandIsNeverShownInFull(t *testing.T) {
	doing, _ := Activity("Bash", json.RawMessage(`{"command":"curl -H 'Authorization: Bearer sk-123' https://x"}`))
	if strings.Contains(doing, "sk-123") || strings.Contains(doing, "Bearer") {
		t.Fatalf("the step shows the command: %q", doing)
	}
}

func toolJSON(session, tool, input string) *strings.Reader {
	return strings.NewReader(`{"session_id":"` + session + `","tool_name":"` + tool + `","tool_input":` + input + `}`)
}

func (f *fakeHook) tool(t *testing.T, session, tool, input string) {
	t.Helper()
	if err := f.Run("tool", toolJSON(session, tool, input)); err != nil {
		t.Fatalf("tool: %v", err)
	}
}

// A working session records the step it is on, how many it has taken and
// how many files it has edited, each file once.
func TestATurnCountsItsStepsAndTheFilesItEdits(t *testing.T) {
	f := newFakeHook(t, 0)
	f.run(t, "prompt", "s1")
	f.tool(t, "s1", "Read", `{"file_path":"/a.go"}`)
	f.tool(t, "s1", "Edit", `{"file_path":"/a.go"}`)
	f.tool(t, "s1", "Edit", `{"file_path":"/a.go"}`)
	f.tool(t, "s1", "Write", `{"file_path":"/b.go"}`)
	f.tool(t, "s1", "Bash", `{"command":"go test","description":"Run the tests"}`)

	s, _ := f.Dir.Get("s1")
	if s.Steps != 5 || len(s.Edits) != 2 || s.Doing != "Run the tests" {
		t.Fatalf("steps %d, edits %d, doing %q", s.Steps, len(s.Edits), s.Doing)
	}

	// The next prompt starts the count again.
	f.now = f.now.Add(time.Minute)
	f.run(t, "prompt", "s1")
	s, _ = f.Dir.Get("s1")
	if s.Steps != 0 || len(s.Edits) != 0 || s.Doing != "" {
		t.Fatalf("a new prompt kept the old turn: %+v", s)
	}
}

// The shim runs the tool event in the background, so the call that needed the
// approval can land just after the approval was asked for. It must not hide
// it; a later call means the approval was given.
func TestAToolCallRightAfterAnApprovalDoesNotHideIt(t *testing.T) {
	f := newFakeHook(t, 0)
	f.run(t, "prompt", "s1")
	f.run(t, "waiting", "s1")
	f.now = f.now.Add(time.Second)
	f.tool(t, "s1", "Bash", `{"command":"rm -rf build"}`)
	if f.state("s1") != Waiting {
		t.Fatalf("state %q, want waiting", f.state("s1"))
	}
	f.now = f.now.Add(10 * time.Second)
	f.tool(t, "s1", "Bash", `{"command":"make"}`)
	if f.state("s1") != Working {
		t.Fatalf("state %q after a later call, want working", f.state("s1"))
	}
}

// A tool event that arrives after the agent finished, or for a session never
// prompted, changes nothing.
func TestALateToolCallChangesNothing(t *testing.T) {
	f := newFakeHook(t, 0)
	f.tool(t, "s0", "Read", `{}`)
	if _, ok := f.Dir.Get("s0"); ok {
		t.Fatal("a tool call made a session out of nothing")
	}
	f.run(t, "prompt", "s1")
	f.run(t, "stop", "s1")
	before, _ := f.Dir.Get("s1")
	f.tool(t, "s1", "Read", `{"file_path":"/a.go"}`)
	after, _ := f.Dir.Get("s1")
	if after.State != Done || after.Steps != before.Steps || !after.At.Equal(before.At) {
		t.Fatalf("a late call changed a finished turn: %+v", after)
	}
}

// Steps while the agent works must not stop the game opening for that prompt.
func TestStepsDuringTheDelayStillOpenTheGame(t *testing.T) {
	f := newFakeHook(t, 20*time.Second)
	f.during = func() { f.tool(t, "s1", "Read", `{"file_path":"/a.go"}`) }
	f.run(t, "prompt", "s1")
	if f.opened != 1 {
		t.Fatalf("opened %d times", f.opened)
	}
}

// A tool call that writes a large file is still read.
func TestALargeToolCallIsStillRead(t *testing.T) {
	f := newFakeHook(t, 0)
	f.run(t, "prompt", "s1")
	big := `{"file_path":"/big.txt","content":"` + strings.Repeat("x", 1<<20) + `"}`
	f.tool(t, "s1", "Write", big)
	if s, _ := f.Dir.Get("s1"); s.Doing != "Editing big.txt" {
		t.Fatalf("doing %q", s.Doing)
	}
}
