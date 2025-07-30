package main

import (
	"time"
)

type LogEntry struct {
	Timestamp   time.Time
	Level       string
	Source      string
	Component   string
	Message     string
	TickNumber  int
	FSMName     string
	FromState   string
	ToState     string
	Event       string
	DesiredState string
	EntryType   EntryType
}

type EntryType int

const (
	EntryTypeUnknown EntryType = iota
	EntryTypeTick
	EntryTypeFSMTransition
	EntryTypeFSMAttempt
	EntryTypeFSMFailed
	EntryTypeFSMDesiredState
	EntryTypeReconciliation
	EntryTypeActionStart
	EntryTypeActionDone
	EntryTypeError
)

type TickData struct {
	Number    int
	Timestamp time.Time
	Events    []LogEntry
}

type FSMHistory struct {
	Name        string
	Transitions []FSMTransition
}

type FSMTransition struct {
	Tick         int
	Timestamp    time.Time
	FromState    string
	ToState      string
	Event        string
	DesiredState string
	Success      bool
}

type LogAnalyzer struct {
	Entries      []LogEntry
	TickMap      map[int]*TickData
	FSMHistories map[string]*FSMHistory
	CurrentTick  int
	Errors       []LogEntry
	Starts       []time.Time
	Sessions     []SessionInfo
}

type SessionInfo struct {
	StartTime time.Time
	EndTime   time.Time
	StartTick int
	EndTick   int
	Errors    int
}