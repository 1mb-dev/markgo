// Package main provides the unified CLI entry point for all MarkGo commands.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	initcmd "github.com/1mb-dev/markgo/internal/commands/init"
	"github.com/1mb-dev/markgo/internal/commands/new"
	"github.com/1mb-dev/markgo/internal/commands/serve"
	"github.com/1mb-dev/markgo/internal/constants"
)

// healthCheckTimeout stays under the Dockerfile HEALTHCHECK --timeout (3s) so the
// probe reports a failure itself rather than being killed mid-request, which reads
// as a timeout rather than an unhealthy service.
const healthCheckTimeout = 2 * time.Second

// defaultHealthCheckPort mirrors the PORT default in internal/config. The probe runs
// as a separate process from the server, so it cannot read the loaded config and has
// to re-derive the port from the same environment the server saw.
const defaultHealthCheckPort = "3000"

func main() {
	// If no arguments provided, default to serve
	if len(os.Args) < 2 {
		serve.Run(os.Args)
		return
	}

	command := os.Args[1]

	// Handle help and version flags
	if command == "-h" || command == "--help" || command == "help" {
		showHelp()
		return
	}

	// Container HEALTHCHECK and docker-compose invoke this. The runtime image is
	// FROM scratch — no shell, no curl — so the binary has to probe itself.
	if command == "--health-check" || command == "health-check" {
		os.Exit(runHealthCheck())
	}

	if command == "-v" || command == "--version" || command == "version" {
		fmt.Printf("MarkGo %s\n", constants.AppVersion)
		if constants.GitCommit != "unknown" {
			fmt.Printf("  commit: %s\n  built:  %s\n", constants.GitCommit, constants.BuildTime)
		}
		return
	}

	// Route to appropriate subcommand
	// Remove the subcommand from args for the subcommand handler
	subArgs := append([]string{os.Args[0]}, os.Args[2:]...)

	switch command {
	case "serve", "server", "start":
		serve.Run(subArgs)
	case "init", "initialize":
		initcmd.Run(subArgs)
	case "new", "create", "article":
		new.Run(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		showHelp()
		os.Exit(1)
	}
}

// runHealthCheck probes the local server's /health endpoint and returns a process
// exit code: 0 healthy, 1 otherwise. The endpoint answers 503 when the article
// service is degraded, so a reachable-but-unhealthy server must still fail here.
func runHealthCheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultHealthCheckPort
	}

	url := "http://" + net.JoinHostPort("127.0.0.1", port) + "/health"
	client := &http.Client{Timeout: healthCheckTimeout}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "health check failed: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "health check failed: %s returned %d\n", url, resp.StatusCode)
		return 1
	}
	return 0
}

func showHelp() {
	fmt.Printf(`MarkGo %s - File-based blog engine

USAGE:
    markgo [command] [flags]

COMMANDS:
    serve       Start the blog server (default)
    init        Initialize a new blog
    new         Create a new article
    version     Show version information
    help        Show this help message

FLAGS:
    --health-check   Probe the running server's /health endpoint; exit 0 if healthy

ALIASES:
    server, start    → serve
    initialize       → init
    create, article  → new

Run 'markgo [command] --help' for more information on a specific command.

EXAMPLES:
    markgo                          # Start server (default command)
    markgo serve                    # Start server explicitly
    markgo serve --help             # Show server options
    markgo init --quick             # Quick blog initialization
    markgo new --title "Hello"      # Create new article

For more information, visit: https://github.com/1mb-dev/markgo
`, constants.AppVersion)
}
