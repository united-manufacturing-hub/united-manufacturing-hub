// Copyright 2025 UMH Systems GmbH
package explorer

import (
	"context"
	"strings"
)

type Scenario interface {
	Setup(ctx context.Context) error
	Tick(ctx context.Context) error
	GetCurrentState() (string, string)
	GetObservedState() interface{}
	GetDesiredState() interface{}
	InjectShutdown() error
	Cleanup(ctx context.Context) error
}

type Explorer struct {
	scenario Scenario
}

func New(scenario Scenario) *Explorer {
	return &Explorer{
		scenario: scenario,
	}
}

func (e *Explorer) Tick(ctx context.Context) error {
	return e.scenario.Tick(ctx)
}

func (e *Explorer) GetCurrentState() (string, string) {
	return e.scenario.GetCurrentState()
}

type CommandType int

const (
	CommandUnknown CommandType = iota
	CommandTick
	CommandShowState
	CommandShowObserved
	CommandShowDesired
	CommandShutdown
	CommandQuit
)

type Command struct {
	Type CommandType
}

type CommandResult struct {
	Success bool
	Message string
}

func ParseCommand(input string) Command {
	input = strings.TrimSpace(input)

	switch input {
	case "t":
		return Command{Type: CommandTick}
	case "s":
		return Command{Type: CommandShowState}
	case "o":
		return Command{Type: CommandShowObserved}
	case "d":
		return Command{Type: CommandShowDesired}
	case "shutdown":
		return Command{Type: CommandShutdown}
	case "q":
		return Command{Type: CommandQuit}
	default:
		return Command{Type: CommandUnknown}
	}
}

func (e *Explorer) ExecuteCommand(ctx context.Context, cmd Command) CommandResult {
	switch cmd.Type {
	case CommandTick:
		err := e.scenario.Tick(ctx)
		if err != nil {
			return CommandResult{Success: false, Message: err.Error()}
		}
		return CommandResult{Success: true, Message: "Tick executed"}

	case CommandShowState:
		state, reason := e.scenario.GetCurrentState()
		return CommandResult{
			Success: true,
			Message: "State: " + state + "\nReason: " + reason,
		}

	case CommandShowObserved:
		observed := e.scenario.GetObservedState()
		return CommandResult{
			Success: true,
			Message: "Observed state: " + formatState(observed),
		}

	case CommandShowDesired:
		desired := e.scenario.GetDesiredState()
		return CommandResult{
			Success: true,
			Message: "Desired state: " + formatState(desired),
		}

	case CommandShutdown:
		err := e.scenario.InjectShutdown()
		if err != nil {
			return CommandResult{Success: false, Message: err.Error()}
		}
		return CommandResult{Success: true, Message: "Shutdown injected"}

	case CommandQuit:
		return CommandResult{Success: true, Message: "Quitting"}

	default:
		return CommandResult{Success: false, Message: "Unknown command"}
	}
}

func formatState(state interface{}) string {
	if state == nil {
		return "<nil>"
	}
	return "<data>"
}
