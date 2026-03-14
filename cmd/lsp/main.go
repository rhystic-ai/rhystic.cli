// cmd/lsp/main.go - Language Server Protocol entry point
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tliron/commonlog"
	_ "github.com/tliron/commonlog/simple"

	"github.com/rhystic/attractor/pkg/lsp"
)

func main() {
	var (
		logLevel = flag.String("log-level", "info", "Log level: debug, info, warn, error")
		noAgent  = flag.Bool("no-agent", false, "Disable agent/LLM features (tree-sitter only)")
		help     = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help {
		fmt.Println("Attractor Language Server")
		fmt.Println()
		fmt.Println("Usage: attractor lsp [options]")
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		os.Exit(0)
	}

	// Configure logging
	commonlog.Configure(0, nil)

	// Ensure log directory exists
	logDir := filepath.Join(os.Getenv("HOME"), ".attractor")
	os.MkdirAll(logDir, 0755)

	// Create and run server
	cfg := lsp.Config{
		LogLevel:    *logLevel,
		EnableAgent: !*noAgent,
		LogFile:     filepath.Join(logDir, "lsp.log"),
	}

	server := lsp.NewServer(cfg)
	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "LSP server error: %v\n", err)
		os.Exit(1)
	}
}
