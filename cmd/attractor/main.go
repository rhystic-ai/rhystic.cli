// Package main provides the Attractor CLI application.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rhystic/attractor/pkg/agent"
	pcontext "github.com/rhystic/attractor/pkg/context"
	"github.com/rhystic/attractor/pkg/dot"
	"github.com/rhystic/attractor/pkg/engine"
	"github.com/rhystic/attractor/pkg/events"
	"github.com/rhystic/attractor/pkg/llm"
	"github.com/rhystic/attractor/pkg/roles"
	"github.com/rhystic/attractor/pkg/store"
	"github.com/rhystic/attractor/pkg/tools"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	// Parse command line
	cmd, opts, err := parseArgs(args)
	if err != nil {
		return err
	}

	switch cmd {
	case "help", "":
		printUsage(stdout)
		return nil
	case "version":
		fmt.Fprintf(stdout, "attractor %s\n", version)
		return nil
	case "run":
		return runPipeline(opts, stdin, stdout, stderr)
	case "agent":
		return runAgent(opts, stdin, stdout, stderr)
	case "validate":
		return validatePipeline(opts, stdout, stderr)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

type options struct {
	dotFile     string
	prompt      string
	model       string
	role        string // Role name for agent command
	logsDir     string
	dbPath      string // SQLite database path; empty disables persistence
	verbose     bool
	noColor     bool
	timeout     time.Duration
	maxRetries  int
	interactive bool
	context     []string // Context values: "key=value" or "@file"
}

func parseArgs(args []string) (string, options, error) {
	// Default DB path: ~/.attractor/attractor.db
	defaultDB := ""
	if home, err := os.UserHomeDir(); err == nil {
		defaultDB = filepath.Join(home, ".attractor", "attractor.db")
	}

	// Allow env var override
	if envDB := os.Getenv("ATTRACTOR_DB"); envDB != "" {
		defaultDB = envDB
	}

	opts := options{
		model:      "minimax/minimax-m2.5",
		logsDir:    "./attractor-logs",
		dbPath:     defaultDB,
		timeout:    30 * time.Minute,
		maxRetries: 50,
	}

	if len(args) == 0 {
		return "help", opts, nil
	}

	cmd := args[0]
	args = args[1:]

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			return "help", opts, nil
		case arg == "-v" || arg == "--version":
			return "version", opts, nil
		case arg == "--verbose":
			opts.verbose = true
		case arg == "--no-color":
			opts.noColor = true
		case arg == "-i" || arg == "--interactive":
			opts.interactive = true
		case arg == "--db":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("--db requires a file path")
			}
			i++
			opts.dbPath = args[i]
		case arg == "--no-db":
			opts.dbPath = ""
		case arg == "-f" || arg == "--file":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("-f requires a file path")
			}
			i++
			opts.dotFile = args[i]
		case arg == "-m" || arg == "--model":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("-m requires a model name")
			}
			i++
			opts.model = args[i]
		case arg == "-r" || arg == "--role":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("-r requires a role name")
			}
			i++
			opts.role = args[i]
		case arg == "-l" || arg == "--logs":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("-l requires a directory path")
			}
			i++
			opts.logsDir = args[i]
		case arg == "-t" || arg == "--timeout":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("-t requires a duration")
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil {
				return "", opts, fmt.Errorf("invalid timeout: %w", err)
			}
			opts.timeout = d
		case arg == "--max-retries":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("--max-retries requires a number")
			}
			i++
			var n int
			if _, err := fmt.Sscanf(args[i], "%d", &n); err != nil {
				return "", opts, fmt.Errorf("invalid max-retries: %w", err)
			}
			opts.maxRetries = n
		case arg == "-c" || arg == "--context":
			if i+1 >= len(args) {
				return "", opts, fmt.Errorf("-c requires a context value (key=value or @file)")
			}
			i++
			opts.context = append(opts.context, args[i])
		case strings.HasPrefix(arg, "-"):
			return "", opts, fmt.Errorf("unknown flag: %s", arg)
		default:
			// Positional argument
			if opts.dotFile == "" && (cmd == "run" || cmd == "validate") {
				opts.dotFile = arg
			} else if opts.prompt == "" && cmd == "agent" {
				opts.prompt = arg
			} else {
				opts.prompt = strings.Join(append([]string{opts.prompt}, args[i:]...), " ")
				break
			}
		}
	}

	return cmd, opts, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `Attractor - DOT-based AI pipeline runner

Usage:
  attractor <command> [options]

Commands:
  run       Execute a DOT pipeline file
  agent     Run the coding agent with a prompt
  validate  Validate a DOT pipeline file
  help      Show this help message
  version   Show version information

Run Options:
  -f, --file <path>      DOT pipeline file to execute
  -m, --model <name>     LLM model (default: minimax/minimax-m2.5)
  -l, --logs <dir>       Logs directory (default: ./attractor-logs)
  -t, --timeout <dur>    Execution timeout (default: 30m)
  --max-retries <n>      Maximum retries per node (default: 50)
  --db <path>            SQLite database path (default: ~/.attractor/attractor.db)
  --no-db                Disable database persistence
  -i, --interactive      Enable interactive mode for human gates
  -c, --context <val>    Inject context (key=value or @file); repeatable
  --verbose              Enable verbose output
  --no-color             Disable colored output

Agent Options:
  -m, --model <name>     LLM model (default: minimax/minimax-m2.5)
  -r, --role <name>      Role for the agent (researcher, project-manager,
                          developer, quality, reviewer, devops)
  -c, --context <val>    Inject context (key=value or @file); repeatable
  --verbose              Enable verbose output

Environment Variables:
  OPENROUTER_API_KEY     OpenRouter API key (required)
  ATTRACTOR_DB           Override default SQLite database path

Examples:
  attractor run pipeline.dot
  attractor agent "Fix the bug in main.go"
  attractor validate workflow.dot

`)
}

func runPipeline(opts options, stdin io.Reader, stdout, stderr io.Writer) error {
	if opts.dotFile == "" {
		return fmt.Errorf("no DOT file specified")
	}

	// Read and parse DOT file
	content, err := os.ReadFile(opts.dotFile)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	graph, err := dot.Parse(string(content))
	if err != nil {
		return fmt.Errorf("parse DOT: %w", err)
	}

	// Create LLM client
	client, err := llm.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Create engine
	cfg := engine.DefaultConfig()
	cfg.LogsRoot = opts.logsDir
	cfg.MaxRetries = opts.maxRetries

	eng := engine.New(graph, client, cfg)
	defer eng.Close()

	// Inject context values into engine context
	if len(opts.context) > 0 {
		ctxValues, err := parseContextValues(opts.context)
		if err != nil {
			return fmt.Errorf("parse context: %w", err)
		}
		for key, value := range ctxValues {
			eng.Context.Set("user."+key, value)
		}
	}

	// Open persistence store (optional)
	if opts.dbPath != "" {
		db, err := store.Open(opts.dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "Warning: could not open database: %v\n", err)
		} else {
			defer db.Close()
			eng.Store = db

			// Subscribe a second channel for persistent event storage
			persistCh := eng.Subscribe()
			go db.PersistEvents(persistCh, eng.Model())
		}
	}

	// Set up event handling
	eventCh := eng.Subscribe()
	go handleEvents(eventCh, stdout, stderr, opts.verbose, opts.noColor)

	// Set up context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(stderr, "\nInterrupted. Shutting down...\n")
		cancel()
	}()

	// Run pipeline
	printHeader(stdout, opts.noColor, "Running pipeline: %s", graph.Name)
	if graph.Goal() != "" {
		fmt.Fprintf(stdout, "Goal: %s\n", graph.Goal())
	}
	fmt.Fprintln(stdout)

	outcome, err := eng.Run(ctx)
	if err != nil {
		return fmt.Errorf("pipeline failed: %w", err)
	}

	printResult(stdout, opts.noColor, outcome)

	// Print usage and cost
	usage := eng.TotalUsage()
	model := eng.Model()
	fmt.Fprintf(stdout, "\nTokens: %d input, %d output, %d total\n",
		usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
	printCost(stdout, usage, model)

	return nil
}

func runAgent(opts options, stdin io.Reader, stdout, stderr io.Writer) error {
	if opts.prompt == "" {
		// Read from stdin
		data, err := io.ReadAll(stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		opts.prompt = strings.TrimSpace(string(data))
		if opts.prompt == "" {
			return fmt.Errorf("no prompt provided")
		}
	}

	// Prepend context to prompt if provided
	if len(opts.context) > 0 {
		ctxValues, err := parseContextValues(opts.context)
		if err != nil {
			return fmt.Errorf("parse context: %w", err)
		}
		// Build context string
		var contextStr string
		for key, value := range ctxValues {
			if key == "unnamed" {
				contextStr = value + "\n\n"
			} else {
				contextStr += fmt.Sprintf("%s: %s\n", key, value)
			}
		}
		if contextStr != "" {
			opts.prompt = contextStr + opts.prompt
		}
	}

	// Create LLM client
	client, err := llm.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Create agent session
	cfg := agent.DefaultConfig()
	cfg.Model = opts.model

	var sessionOpts []agent.SessionOption

	// Load role if specified
	if opts.role != "" {
		role, err := roles.Load(opts.role)
		if err != nil {
			return fmt.Errorf("load role %q: %w", opts.role, err)
		}

		// Set role's system prompt with template expansion
		wd, _ := os.Getwd()
		// Build context string for template expansion
		var contextStr string
		if len(opts.context) > 0 {
			ctxValues, _ := parseContextValues(opts.context)
			for key, value := range ctxValues {
				if key == "unnamed" {
					contextStr = value + "\n" + contextStr
				} else {
					contextStr += fmt.Sprintf("%s: %s\n", key, value)
				}
			}
		}
		if expanded, err := role.ExpandPrompt(wd, contextStr); err == nil && expanded != "" {
			cfg.SystemPrompt = expanded
		}

		// Build restricted tool registry
		reg := tools.RegistryForRole(role)
		sessionOpts = append(sessionOpts, agent.WithToolRegistry(reg))

		if opts.verbose {
			fmt.Fprintf(stderr, "Role: %s (%s)\n", role.Name, role.Description)
			fmt.Fprintf(stderr, "Tools: %v\n", reg.Names())
		}
	}

	session := agent.NewSession(client, cfg, sessionOpts...)

	// Open persistence store (optional)
	var db *store.Store
	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())
	if opts.dbPath != "" {
		var err error
		db, err = store.Open(opts.dbPath)
		if err != nil {
			fmt.Fprintf(stderr, "Warning: could not open database: %v\n", err)
		} else {
			defer db.Close()
			_ = db.CreateRun(store.Run{
				ID:        runID,
				Mode:      "agent",
				Model:     opts.model,
				StartedAt: time.Now(),
			})

			// Subscribe for persistent event storage
			persistCh := session.Events.Subscribe()
			go db.PersistEvents(persistCh, opts.model)
		}
	}

	// Set up event handling
	eventCh := session.Events.Subscribe()
	go handleEvents(eventCh, stdout, stderr, opts.verbose, opts.noColor)

	// Set up context
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(stderr, "\nInterrupted. Aborting...\n")
		session.Abort()
		cancel()
	}()

	printHeader(stdout, opts.noColor, "Running agent with model: %s", opts.model)
	if opts.role != "" {
		fmt.Fprintf(stdout, "Role: %s\n", opts.role)
	}
	fmt.Fprintln(stdout)

	if err := session.Submit(ctx, opts.prompt); err != nil {
		return fmt.Errorf("agent failed: %w", err)
	}

	// Print final response
	response := session.LastResponse()
	if response != "" {
		fmt.Fprintln(stdout)
		printSection(stdout, opts.noColor, "Response")
		fmt.Fprintln(stdout, response)
	}

	// Persist conversation and artifacts to store
	if db != nil {
		_ = db.InsertArtifact(runID, "agent", "prompt", opts.prompt)
		if response != "" {
			_ = db.InsertArtifact(runID, "agent", "response", response)
		}

		for i, turn := range session.History {
			ct := store.ConversationTurn{
				RunID:     runID,
				NodeID:    "agent",
				TurnIndex: i,
				Role:      string(turn.Role),
				Content:   turn.Content,
				Timestamp: turn.Timestamp,
			}
			if len(turn.ToolCalls) > 0 {
				if b, err := json.Marshal(turn.ToolCalls); err == nil {
					ct.ToolCalls = string(b)
				}
			}
			if len(turn.Results) > 0 {
				if b, err := json.Marshal(turn.Results); err == nil {
					ct.ToolResults = string(b)
				}
			}
			_ = db.InsertConversationTurn(ct)
		}
	}

	// Print usage and cost
	usage := session.TotalUsage()
	model := session.Model()
	fmt.Fprintf(stdout, "\nTokens: %d input, %d output, %d total\n",
		usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
	printCost(stdout, usage, model)

	// Persist run completion
	if db != nil {
		total, _, _ := usage.Cost(model)
		_ = db.UpdateRun(runID, store.RunUpdate{
			Status:            "success",
			EndedAt:           time.Now(),
			DurationMs:        0, // Agent mode doesn't track pipeline duration
			TotalInputTokens:  usage.InputTokens,
			TotalOutputTokens: usage.OutputTokens,
			TotalCostUSD:      total,
		})
	}

	return nil
}

func validatePipeline(opts options, stdout, stderr io.Writer) error {
	if opts.dotFile == "" {
		return fmt.Errorf("no DOT file specified")
	}

	// Read and parse DOT file
	content, err := os.ReadFile(opts.dotFile)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	graph, err := dot.Parse(string(content))
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	// Validate
	var errors []string
	var warnings []string

	// Check for start node
	if graph.FindStartNode() == nil {
		errors = append(errors, "No start node found (shape=Mdiamond)")
	}

	// Check for exit node
	if graph.FindExitNode() == nil {
		errors = append(errors, "No exit node found (shape=Msquare)")
	}

	// Check for unreachable nodes
	reachable := make(map[string]bool)
	if start := graph.FindStartNode(); start != nil {
		markReachable(graph, start.ID, reachable)
	}
	for id := range graph.Nodes {
		if !reachable[id] {
			warnings = append(warnings, fmt.Sprintf("Node '%s' is unreachable", id))
		}
	}

	// Check for edges to non-existent nodes
	for _, edge := range graph.Edges {
		if _, ok := graph.Nodes[edge.From]; !ok {
			errors = append(errors, fmt.Sprintf("Edge source '%s' not found", edge.From))
		}
		if _, ok := graph.Nodes[edge.To]; !ok {
			errors = append(errors, fmt.Sprintf("Edge target '%s' not found", edge.To))
		}
	}

	// Validate role attributes
	var roleNodes []string
	for id, node := range graph.Nodes {
		roleName := node.Role()
		if roleName == "" {
			continue
		}
		if _, err := roles.Load(roleName); err != nil {
			warnings = append(warnings, fmt.Sprintf("Node '%s' references unknown role %q", id, roleName))
		} else {
			roleNodes = append(roleNodes, id)
		}
	}

	// Print results
	fmt.Fprintf(stdout, "Validating: %s\n\n", filepath.Base(opts.dotFile))

	if len(errors) > 0 {
		fmt.Fprintf(stdout, "Errors (%d):\n", len(errors))
		for _, e := range errors {
			fmt.Fprintf(stdout, "  - %s\n", e)
		}
		fmt.Fprintln(stdout)
	}

	if len(warnings) > 0 {
		fmt.Fprintf(stdout, "Warnings (%d):\n", len(warnings))
		for _, w := range warnings {
			fmt.Fprintf(stdout, "  - %s\n", w)
		}
		fmt.Fprintln(stdout)
	}

	if len(errors) == 0 {
		fmt.Fprintf(stdout, "Graph: %s\n", graph.Name)
		fmt.Fprintf(stdout, "Nodes: %d\n", len(graph.Nodes))
		fmt.Fprintf(stdout, "Edges: %d\n", len(graph.Edges))
		if graph.Goal() != "" {
			fmt.Fprintf(stdout, "Goal: %s\n", graph.Goal())
		}
		if len(roleNodes) > 0 {
			fmt.Fprintf(stdout, "Roles: %d node(s) with role assignments\n", len(roleNodes))
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Validation passed!")
		return nil
	}

	return fmt.Errorf("validation failed with %d error(s)", len(errors))
}

func markReachable(graph *dot.Graph, nodeID string, reachable map[string]bool) {
	if reachable[nodeID] {
		return
	}
	reachable[nodeID] = true
	for _, edge := range graph.OutgoingEdges(nodeID) {
		markReachable(graph, edge.To, reachable)
	}
}

func handleEvents(ch <-chan events.Event, stdout, stderr io.Writer, verbose, noColor bool) {
	for event := range ch {
		switch event.Type {
		case events.EventNodeStart:
			if verbose {
				printEvent(stdout, noColor, "node", "Starting: %s (%s)",
					event.Data.NodeLabel, event.Data.NodeType)
			}
		case events.EventNodeEnd:
			if event.Data.Status == "fail" {
				printEvent(stderr, noColor, "error", "Failed: %s - %s",
					event.NodeID, event.Data.FailureReason)
			} else if verbose {
				printEvent(stdout, noColor, "success", "Completed: %s",
					event.NodeID)
			}
		case events.EventNodeRetry:
			printEvent(stdout, noColor, "retry", "Retrying %s (attempt %d/%d)",
				event.NodeID, event.Data.AttemptNum, event.Data.MaxAttempts)
		case events.EventLLMStart:
			if verbose {
				printEvent(stdout, noColor, "llm", "Calling %s...", event.Data.Model)
			}
		case events.EventLLMDelta:
			fmt.Fprint(stdout, event.Data.Delta)
		case events.EventLLMEnd:
			if verbose {
				printEvent(stdout, noColor, "llm", "Received %d tokens",
					event.Data.OutputTokens)
			}
		case events.EventToolStart:
			printEvent(stdout, noColor, "tool", "Running %s", event.Data.ToolName)
			if verbose && event.Data.ToolArgs != "" {
				fmt.Fprintf(stdout, "  Args: %s\n", truncateString(event.Data.ToolArgs, 100))
			}
		case events.EventToolEnd:
			if event.Data.IsError {
				printEvent(stderr, noColor, "error", "Tool failed: %s", event.Data.ToolOutput)
			} else if verbose {
				printEvent(stdout, noColor, "tool", "Tool completed")
				if event.Data.ToolOutput != "" {
					output := truncateString(event.Data.ToolOutput, 200)
					fmt.Fprintf(stdout, "  Output: %s\n", output)
				}
			}
		case events.EventHumanWaiting:
			printEvent(stdout, noColor, "human", "%s", event.Data.Question)
			for i, opt := range event.Data.Options {
				fmt.Fprintf(stdout, "  [%d] %s\n", i+1, opt)
			}
		case events.EventHumanResponse:
			printEvent(stdout, noColor, "human", "Selected: %s", event.Data.Selected)
		case events.EventLog:
			if verbose || event.Data.Level == "error" || event.Data.Level == "warn" {
				printEvent(stdout, noColor, event.Data.Level, "%s", event.Data.Message)
			}
		case events.EventPipelineError:
			printEvent(stderr, noColor, "error", "%s", event.Data.Error)
		case events.EventEdgeSelected:
			if verbose {
				if event.Data.EdgeLabel != "" {
					printEvent(stdout, noColor, "edge", "%s -> %s (%s)",
						event.Data.FromNode, event.Data.ToNode, event.Data.EdgeLabel)
				} else {
					printEvent(stdout, noColor, "edge", "%s -> %s",
						event.Data.FromNode, event.Data.ToNode)
				}
			}
		case events.EventNodeSkip:
			if verbose {
				printEvent(stdout, noColor, "skip", "Skipping: %s (%s)",
					event.Data.NodeLabel, event.Data.Notes)
			}
		case events.EventEdgeEvaluated:
			if verbose {
				met := "false"
				if event.Data.ConditionMet {
					met = "true"
				}
				printEvent(stdout, noColor, "edge", "Evaluated: %s -> %s [%s] = %s",
					event.Data.FromNode, event.Data.ToNode, event.Data.Condition, met)
			}
		case events.EventLLMError:
			printEvent(stderr, noColor, "error", "LLM error: %s", event.Data.Error)
		case events.EventToolOutput:
			if verbose {
				output := truncateString(event.Data.ToolOutput, 200)
				printEvent(stdout, noColor, "tool", "Output from %s: %s",
					event.Data.ToolName, output)
			}
		case events.EventToolError:
			printEvent(stderr, noColor, "error", "Tool error (%s): %s",
				event.Data.ToolName, event.Data.Error)
		case events.EventHumanTimeout:
			printEvent(stdout, noColor, "human", "Timeout: defaulting to %s",
				event.Data.Selected)
		case events.EventCheckpoint:
			if verbose {
				printEvent(stdout, noColor, "checkpoint", "Saved checkpoint at %s",
					event.NodeID)
			}
		case events.EventLoopDetected:
			printEvent(stderr, noColor, "warn", "Loop detected: %s",
				event.Data.Message)
		case events.EventGoalGateCheck:
			if verbose {
				printEvent(stdout, noColor, "gate", "Goal gate check: %s (%s)",
					event.NodeID, event.Data.Status)
			}
		case events.EventGoalGateFail:
			printEvent(stderr, noColor, "error", "Goal gate failed: %s - %s",
				event.NodeID, event.Data.FailureReason)
		}
	}
}

func printHeader(w io.Writer, noColor bool, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if noColor {
		fmt.Fprintf(w, "=== %s ===\n", msg)
	} else {
		fmt.Fprintf(w, "\033[1;36m=== %s ===\033[0m\n", msg)
	}
}

func printSection(w io.Writer, noColor bool, title string) {
	if noColor {
		fmt.Fprintf(w, "--- %s ---\n", title)
	} else {
		fmt.Fprintf(w, "\033[1;33m--- %s ---\033[0m\n", title)
	}
}

func printEvent(w io.Writer, noColor bool, level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05")

	if noColor {
		fmt.Fprintf(w, "[%s] [%s] %s\n", timestamp, level, msg)
		return
	}

	var color string
	switch level {
	case "error":
		color = "\033[31m" // Red
	case "warn":
		color = "\033[33m" // Yellow
	case "success":
		color = "\033[32m" // Green
	case "llm":
		color = "\033[35m" // Magenta
	case "tool":
		color = "\033[34m" // Blue
	case "human":
		color = "\033[36m" // Cyan
	case "retry":
		color = "\033[33m" // Yellow
	default:
		color = "\033[37m" // White
	}

	fmt.Fprintf(w, "\033[90m[%s]\033[0m %s[%s]\033[0m %s\n", timestamp, color, level, msg)
}

func printResult(w io.Writer, noColor bool, outcome pcontext.Outcome) {
	fmt.Fprintln(w)
	switch outcome.Status {
	case pcontext.StatusSuccess:
		if noColor {
			fmt.Fprintln(w, "Pipeline completed successfully!")
		} else {
			fmt.Fprintln(w, "\033[32mPipeline completed successfully!\033[0m")
		}
	case pcontext.StatusPartialSuccess:
		if noColor {
			fmt.Fprintln(w, "Pipeline completed with partial success.")
		} else {
			fmt.Fprintln(w, "\033[33mPipeline completed with partial success.\033[0m")
		}
	case pcontext.StatusFail:
		if noColor {
			fmt.Fprintf(w, "Pipeline failed: %s\n", outcome.FailureReason)
		} else {
			fmt.Fprintf(w, "\033[31mPipeline failed: %s\033[0m\n", outcome.FailureReason)
		}
	}

	if outcome.Notes != "" {
		fmt.Fprintf(w, "Notes: %s\n", outcome.Notes)
	}
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func printCost(w io.Writer, usage llm.Usage, model string) {
	total, inputCost, outputCost := usage.Cost(model)
	if llm.HasPricing(model) {
		fmt.Fprintf(w, "Cost: $%.4f (input: $%.4f, output: $%.4f)\n", total, inputCost, outputCost)
	} else {
		fmt.Fprintf(w, "Cost: $0.0000 (pricing unavailable for %s)\n", model)
	}
}

// resolveContextValue resolves a context value.
// If the value starts with '@', it reads the file contents.
// Otherwise, it returns the literal string.
func resolveContextValue(val string) (string, error) {
	if strings.HasPrefix(val, "@") {
		filePath := strings.TrimSpace(val[1:])
		if filePath == "" {
			return "", fmt.Errorf("empty file path after @")
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read context file %q: %w", filePath, err)
		}
		return string(content), nil
	}
	return val, nil
}

// parseContextValues parses context flags into a map of key->value.
// Format: "key=value" or just "@file" (stored under "unnamed" key)
func parseContextValues(contextFlags []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, flag := range contextFlags {
		if strings.HasPrefix(flag, "@") {
			// File reference - read and store under "unnamed"
			content, err := resolveContextValue(flag)
			if err != nil {
				return nil, err
			}
			result["unnamed"] = content
		} else if idx := strings.Index(flag, "="); idx > 0 {
			// key=value format
			key := strings.TrimSpace(flag[:idx])
			value := strings.TrimSpace(flag[idx+1:])
			if key == "" {
				return nil, fmt.Errorf("empty key in context: %q", flag)
			}
			result[key] = value
		} else {
			// Treat as key=value without equals - store as-is
			result[flag] = ""
		}
	}
	return result, nil
}
