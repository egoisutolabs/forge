package bash

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSanitizeEnvFrom_StripsDangerousVars(t *testing.T) {
	dangerous := []string{
		"BASH_ENV=/tmp/evil.sh",
		"ENV=/tmp/evil.sh",
		"CDPATH=/tmp:/var",
		"GLOBIGNORE=*.go",
		"PROMPT_COMMAND=echo pwned",
		"SHELLOPTS=xtrace",
		"BASHOPTS=extglob",
	}
	// Mix in one safe var to confirm it passes
	input := append(dangerous, "PATH=/usr/bin")

	result := sanitizeEnvFrom(input)

	for _, entry := range result {
		name := entry[:strings.IndexByte(entry, '=')]
		if dangerousEnvVars[name] {
			t.Errorf("dangerous var %q should have been stripped", name)
		}
	}

	found := false
	for _, entry := range result {
		if strings.HasPrefix(entry, "PATH=") {
			found = true
		}
	}
	if !found {
		t.Error("safe var PATH should have passed through")
	}
}

func TestSanitizeEnvFrom_StripsBashFuncVars(t *testing.T) {
	input := []string{
		"BASH_FUNC_evil%%=() { echo pwned; }",
		"BASH_FUNC_another%%=() { /bin/sh; }",
		"HOME=/home/user",
	}

	result := sanitizeEnvFrom(input)

	for _, entry := range result {
		if strings.HasPrefix(entry, "BASH_FUNC_") {
			t.Errorf("BASH_FUNC_ var should have been stripped: %q", entry)
		}
	}

	found := false
	for _, entry := range result {
		if strings.HasPrefix(entry, "HOME=") {
			found = true
		}
	}
	if !found {
		t.Error("HOME should have passed through")
	}
}

func TestSanitizeEnvFrom_SafeVarsPassThrough(t *testing.T) {
	safeEntries := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/home/user",
		"USER=testuser",
		"LANG=en_US.UTF-8",
		"TERM=xterm-256color",
		"GOPATH=/home/user/go",
		"EDITOR=vim",
		"SSH_AUTH_SOCK=/tmp/ssh-agent",
		"XDG_CONFIG_HOME=/home/user/.config",
		"ANTHROPIC_API_KEY=sk-ant-test",
	}

	result := sanitizeEnvFrom(safeEntries)

	if len(result) != len(safeEntries) {
		t.Errorf("expected all %d safe vars to pass through, got %d", len(safeEntries), len(result))
		for _, e := range safeEntries {
			found := false
			for _, r := range result {
				if r == e {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("  missing: %s", e[:strings.IndexByte(e, '=')])
			}
		}
	}
}

func TestSanitizeEnvFrom_UnknownVarsDropped(t *testing.T) {
	input := []string{
		"PATH=/usr/bin",
		"SOME_RANDOM_VAR=value",
		"MY_SECRET_TOKEN=abc123",
	}

	result := sanitizeEnvFrom(input)

	if len(result) != 1 {
		t.Errorf("expected 1 var (PATH), got %d: %v", len(result), result)
	}
}

func TestSanitizeEnvFrom_MalformedEntriesSkipped(t *testing.T) {
	input := []string{
		"NOEQUALS",
		"=startswitheq",
		"PATH=/usr/bin",
	}

	result := sanitizeEnvFrom(input)

	if len(result) != 1 {
		t.Errorf("expected 1 var (PATH), got %d: %v", len(result), result)
	}
}

func TestSanitizedEnv_ReturnsSubsetOfOsEnviron(t *testing.T) {
	result := SanitizedEnv()
	osEnv := os.Environ()

	// Result should never be larger than os.Environ()
	if len(result) > len(osEnv) {
		t.Errorf("SanitizedEnv returned %d vars, but os.Environ has %d", len(result), len(osEnv))
	}

	// Every entry in result must exist in os.Environ()
	osSet := make(map[string]bool, len(osEnv))
	for _, e := range osEnv {
		osSet[e] = true
	}
	for _, e := range result {
		if !osSet[e] {
			t.Errorf("SanitizedEnv returned entry not in os.Environ: %q", e)
		}
	}
}

func TestExecCommand_BASH_ENV_DoesNotExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	// Set BASH_ENV in our process environment — this should NOT be inherited
	// by the child command if env sanitization is working.
	payloadFile := t.TempDir() + "/bash_env_payload.sh"
	markerFile := t.TempDir() + "/bash_env_marker"

	// Write a payload that creates a marker file
	if err := os.WriteFile(payloadFile, []byte("touch "+markerFile+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Set BASH_ENV in the current process (will be sanitized by SanitizedEnv)
	os.Setenv("BASH_ENV", payloadFile)
	defer os.Unsetenv("BASH_ENV")

	// Run a simple command — BASH_ENV should NOT be sourced
	result := ExecCommand(context.Background(), "echo safe", ExecOptions{})

	if result.ExitCode != 0 {
		t.Errorf("command failed with exit code %d: %s", result.ExitCode, result.Stdout)
	}

	// The marker file should NOT exist
	if _, err := os.Stat(markerFile); err == nil {
		t.Error("BASH_ENV payload was executed — environment was not sanitized!")
	}
}

func TestStartBackground_BASH_ENV_DoesNotExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	payloadFile := t.TempDir() + "/bash_env_payload.sh"
	markerFile := t.TempDir() + "/bash_env_marker_bg"

	if err := os.WriteFile(payloadFile, []byte("touch "+markerFile+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	os.Setenv("BASH_ENV", payloadFile)
	defer os.Unsetenv("BASH_ENV")

	task, err := StartBackground("echo safe-bg", ExecOptions{})
	if err != nil {
		t.Fatalf("StartBackground failed: %v", err)
	}
	t.Cleanup(func() { os.Remove(task.OutputFile) })

	waitForStatus(task, 5*time.Second)

	if _, err := os.Stat(markerFile); err == nil {
		t.Error("BASH_ENV payload was executed in background task — environment was not sanitized!")
	}
}
