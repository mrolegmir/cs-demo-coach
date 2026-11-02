package coach

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Codex is a Provider that runs the OpenAI Codex CLI (`codex exec`), so the
// coach can use a ChatGPT subscription through the official client instead
// of an API key. Run `codex login` once on the machine first.
//
// The CLI runs read-only in an empty temporary directory, without session
// files, and only its final message is used.
type Codex struct {
	// Binary is the codex executable; empty means "codex" on PATH.
	Binary string
	// Model overrides the CLI's default model; empty keeps the default.
	Model string
}

func (c Codex) Complete(ctx context.Context, system, user string) (string, error) {
	dir, err := os.MkdirTemp("", "csdc-codex-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	out := filepath.Join(dir, "reply.txt")

	args := []string{
		"exec",
		"--skip-git-repo-check",
		"--ephemeral",
		"--sandbox", "read-only",
		"--color", "never",
		"--cd", dir,
		"--output-last-message", out,
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	args = append(args, "-") // prompt from stdin

	bin := c.Binary
	if bin == "" {
		bin = "codex"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	// codex exec has its own agent instructions; ours go first in the prompt.
	cmd.Stdin = strings.NewReader(system + "\n\n" + user)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("codex exec: %w: %s", err, lastLines(stderr.String(), 5))
	}
	reply, err := os.ReadFile(out)
	if err != nil {
		return "", fmt.Errorf("codex exec: no reply: %w", err)
	}
	return string(reply), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], " | ")
}
