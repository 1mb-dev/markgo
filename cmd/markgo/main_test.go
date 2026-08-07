package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runMarkgo runs the CLI via `go run` and returns stdout, stderr, and exit code.
func runMarkgo(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmdArgs := append([]string{"run", "."}, args...)
	cmd := exec.Command("go", cmdArgs...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run markgo: %v", err)
		}
	}

	return outBuf.String(), errBuf.String(), exitCode
}

func TestHelpOutput(t *testing.T) {
	for _, arg := range []string{"help", "--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			stdout, _, exitCode := runMarkgo(t, arg)
			if exitCode != 0 {
				t.Errorf("markgo %s exited with code %d, want 0", arg, exitCode)
			}
			for _, want := range []string{"USAGE:", "COMMANDS:", "ALIASES:", "EXAMPLES:", "serve", "init", "new"} {
				if !strings.Contains(stdout, want) {
					t.Errorf("markgo %s output missing %q", arg, want)
				}
			}
		})
	}
}

func TestVersionOutput(t *testing.T) {
	for _, arg := range []string{"version", "--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			stdout, _, exitCode := runMarkgo(t, arg)
			if exitCode != 0 {
				t.Errorf("markgo %s exited with code %d, want 0", arg, exitCode)
			}
			if !strings.Contains(stdout, "MarkGo ") {
				t.Errorf("markgo %s output missing version string, got: %q", arg, stdout)
			}
		})
	}
}

// healthStub serves /health with the given status on a real port, standing in for a
// running markgo. Returns the port so the probe can be pointed at it via PORT.
func healthStub(t *testing.T, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("could not derive port from %q: %v", srv.URL, err)
	}
	return port
}

// runMarkgoEnv runs the CLI with extra environment entries.
func runMarkgoEnv(t *testing.T, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmdArgs := append([]string{"run", "."}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Env = append(os.Environ(), env...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run markgo: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestHealthCheckHealthy(t *testing.T) {
	port := healthStub(t, http.StatusOK)
	for _, arg := range []string{"--health-check", "health-check"} {
		t.Run(arg, func(t *testing.T) {
			_, stderr, exitCode := runMarkgoEnv(t, []string{"PORT=" + port}, arg)
			if exitCode != 0 {
				t.Errorf("markgo %s exited with code %d, want 0 (stderr: %q)", arg, exitCode, stderr)
			}
		})
	}
}

// A reachable server reporting itself degraded must still fail the probe — otherwise
// the container reads healthy while the article service is down.
func TestHealthCheckUnhealthyStatus(t *testing.T) {
	port := healthStub(t, http.StatusServiceUnavailable)
	_, stderr, exitCode := runMarkgoEnv(t, []string{"PORT=" + port}, "--health-check")
	if exitCode != 1 {
		t.Errorf("exited with code %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "503") {
		t.Errorf("stderr missing the status code, got: %q", stderr)
	}
}

func TestHealthCheckServerDown(t *testing.T) {
	// Bind and immediately release a port so nothing is listening on it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not reserve a port: %v", err)
	}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	_ = l.Close()

	_, stderr, exitCode := runMarkgoEnv(t, []string{"PORT=" + port}, "--health-check")
	if exitCode != 1 {
		t.Errorf("exited with code %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "health check failed") {
		t.Errorf("stderr missing failure message, got: %q", stderr)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, stderr, exitCode := runMarkgo(t, "nonsense")
	if exitCode != 1 {
		t.Errorf("markgo nonsense exited with code %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "Unknown command: nonsense") {
		t.Errorf("markgo nonsense stderr missing error message, got: %q", stderr)
	}
}

func TestServeHelp(t *testing.T) {
	stdout, _, exitCode := runMarkgo(t, "serve", "--help")
	if exitCode != 0 {
		t.Errorf("markgo serve --help exited with code %d, want 0", exitCode)
	}
	for _, want := range []string{"markgo serve", "--port", "CONFIGURATION:", ".env"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("markgo serve --help output missing %q", want)
		}
	}
}

func TestInitHelp(t *testing.T) {
	stdout, _, exitCode := runMarkgo(t, "init", "--help")
	if exitCode != 0 {
		t.Errorf("markgo init --help exited with code %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "--quick") {
		t.Errorf("markgo init --help output missing --quick flag")
	}
}

func TestNewHelp(t *testing.T) {
	stdout, _, exitCode := runMarkgo(t, "new", "--help")
	if exitCode != 0 {
		t.Errorf("markgo new --help exited with code %d, want 0", exitCode)
	}
	for _, want := range []string{"--title", "--template", "AVAILABLE TEMPLATES:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("markgo new --help output missing %q", want)
		}
	}
}
