package coach

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
)

// TestMain lets the test binary act as a fake codex CLI when
// CSDC_FAKE_CODEX is set: it checks the arguments, reads the prompt from
// stdin and writes CSDC_FAKE_CODEX as the reply.
func TestMain(m *testing.M) {
	if reply, ok := os.LookupEnv("CSDC_FAKE_CODEX"); ok {
		os.Exit(fakeCodex(reply))
	}
	os.Exit(m.Run())
}

func fakeCodex(reply string) int {
	args := os.Args[1:]
	if len(args) == 0 || args[0] != "exec" || !slices.Contains(args, "read-only") || args[len(args)-1] != "-" {
		fmt.Fprint(os.Stderr, "unexpected args: "+strings.Join(args, " "))
		return 2
	}
	prompt, _ := io.ReadAll(os.Stdin)
	if !strings.Contains(string(prompt), "SYSTEM") || !strings.Contains(string(prompt), "USER") {
		fmt.Fprint(os.Stderr, "prompt is missing system or user part")
		return 3
	}
	if reply == "fail" {
		fmt.Fprint(os.Stderr, "Not logged in")
		return 1
	}
	i := slices.Index(args, "--output-last-message")
	if err := os.WriteFile(args[i+1], []byte(reply), 0o600); err != nil {
		return 4
	}
	return 0
}

func TestCodex(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := Codex{Binary: self, Model: "gpt-x"}

	t.Setenv("CSDC_FAKE_CODEX", `{"ok":true}`)
	reply, err := c.Complete(t.Context(), "SYSTEM", "USER")
	if err != nil {
		t.Fatal(err)
	}
	if reply != `{"ok":true}` {
		t.Errorf("reply = %q", reply)
	}

	t.Setenv("CSDC_FAKE_CODEX", "fail")
	_, err = c.Complete(t.Context(), "SYSTEM", "USER")
	if err == nil || !strings.Contains(err.Error(), "Not logged in") {
		t.Errorf("error = %v, want the CLI's message", err)
	}
}
