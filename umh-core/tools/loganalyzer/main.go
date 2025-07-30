package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	var (
		logFile     = flag.String("f", "", "Path to the log file to analyze")
		interactive = flag.Bool("i", false, "Run in interactive mode")
		command     = flag.String("c", "", "Command to run (summary, timeline, fsm, list-fsms, stuck)")
		fsmName     = flag.String("fsm", "", "FSM name for fsm command")
		startTick   = flag.Int("start", 1, "Start tick for timeline")
		endTick     = flag.Int("end", 100, "End tick for timeline")
	)
	
	flag.Parse()

	if *logFile == "" {
		fmt.Println("Error: Log file path is required (-f flag)")
		flag.Usage()
		os.Exit(1)
	}

	analyzer, err := ParseLogFile(*logFile)
	if err != nil {
		fmt.Printf("Error parsing log file: %v\n", err)
		os.Exit(1)
	}

	if *interactive {
		runInteractive(analyzer)
	} else if *command != "" {
		runCommand(analyzer, *command, *fsmName, *startTick, *endTick)
	} else {
		analyzer.ShowSummary()
	}
}

func runCommand(analyzer *LogAnalyzer, command, fsmName string, startTick, endTick int) {
	switch command {
	case "summary":
		analyzer.ShowSummary()
	case "timeline":
		analyzer.ShowTickTimeline(startTick, endTick)
	case "fsm":
		if fsmName == "" {
			fmt.Println("Error: FSM name required for fsm command (-fsm flag)")
			return
		}
		analyzer.ShowFSMHistory(fsmName)
	case "list-fsms":
		analyzer.ListFSMs()
	case "stuck":
		analyzer.FindStuckFSMs()
	case "errors":
		analyzer.ShowErrors()
	case "recovery":
		analyzer.AnalyzeRecovery()
	case "orphans":
		analyzer.AnalyzeOrphanedServices()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Available commands: summary, timeline, fsm, list-fsms, stuck, errors, recovery, orphans")
	}
}

func runInteractive(analyzer *LogAnalyzer) {
	reader := bufio.NewReader(os.Stdin)
	
	fmt.Println("\n=== UMH Core Log Analyzer ===")
	fmt.Println("Type 'help' for available commands or 'quit' to exit\n")

	for {
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		command := parts[0]

		switch command {
		case "help":
			showHelp()
			
		case "quit", "exit":
			fmt.Println("Goodbye!")
			return
			
		case "summary":
			analyzer.ShowSummary()
			
		case "timeline":
			start, end := 1, 100
			if len(parts) >= 3 {
				start, _ = strconv.Atoi(parts[1])
				end, _ = strconv.Atoi(parts[2])
			}
			analyzer.ShowTickTimeline(start, end)
			
		case "fsm":
			if len(parts) < 2 {
				fmt.Println("Usage: fsm <fsm-name>")
				continue
			}
			analyzer.ShowFSMHistory(parts[1])
			
		case "list-fsms", "list":
			analyzer.ListFSMs()
			
		case "stuck":
			analyzer.FindStuckFSMs()
			
		case "filter":
			if len(parts) < 2 {
				fmt.Println("Usage: filter <fsm-pattern>")
				continue
			}
			filterFSMs(analyzer, parts[1])
			
		case "visual":
			start, end := 1, 20
			if len(parts) >= 3 {
				start, _ = strconv.Atoi(parts[1])
				end, _ = strconv.Atoi(parts[2])
			}
			analyzer.ShowVisualTimeline(start, end)
			
		case "concurrent":
			analyzer.ShowConcurrentActivity()
			
		case "errors":
			analyzer.ShowErrors()
			
		case "errors-tick":
			if len(parts) < 2 {
				fmt.Println("Usage: errors-tick <tick-number>")
				continue
			}
			tick, _ := strconv.Atoi(parts[1])
			analyzer.ShowErrorsForTick(tick)
			
		case "recovery":
			analyzer.AnalyzeRecovery()
			
		case "compare-sessions":
			analyzer.CompareSessionStates()
			
		case "orphans":
			analyzer.AnalyzeOrphanedServices()
			
		case "instance-timeline":
			analyzer.ShowInstanceNotFoundTimeline()
			
		case "analyze-issue":
			analyzer.AnalyzeRecoveryIssue()
			
		default:
			fmt.Printf("Unknown command: %s\n", command)
			fmt.Println("Type 'help' for available commands")
		}
	}
}

func showHelp() {
	fmt.Println(`
Available Commands:
  summary              - Show overall analysis summary
  timeline [start end] - Show tick timeline (default: 1-100)
  fsm <name>          - Show history of a specific FSM
  list-fsms           - List all FSMs
  stuck               - Find potentially stuck FSMs
  filter <pattern>    - Filter FSMs by name pattern
  visual [start end]  - Show visual FSM timeline (default: 1-20)
  concurrent          - Show concurrent FSM activity analysis
  errors              - Show error analysis and grouping
  errors-tick <tick>  - Show errors at a specific tick
  recovery            - Analyze FSM recovery after restart
  compare-sessions    - Compare FSM states between sessions
  orphans             - Analyze orphaned S6 services
  instance-timeline   - Show timeline of instance not found errors
  help                - Show this help
  quit                - Exit the program
`)
}

func filterFSMs(analyzer *LogAnalyzer, pattern string) {
	fmt.Printf("\n=== FSMs matching '%s' ===\n", pattern)
	
	found := false
	for name := range analyzer.FSMHistories {
		if strings.Contains(name, pattern) {
			found = true
			fmt.Printf("  • %s\n", name)
		}
	}
	
	if !found {
		fmt.Println("No FSMs found matching the pattern")
	}
}