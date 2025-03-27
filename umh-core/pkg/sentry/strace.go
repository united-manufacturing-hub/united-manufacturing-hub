package sentry

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/DataDog/gostackparse"
	"github.com/getsentry/sentry-go"
)

// captureGoroutinesAsThreads captures all current goroutines and converts them to Sentry threads.
func captureGoroutinesAsThreads() ([]sentry.Thread, []byte) {
	// Capture the current stack trace for all goroutines
	stack := entireStack()

	// Parse the stack trace using gostackparse
	goroutines, err := gostackparse.Parse(bytes.NewReader(stack))
	if err != nil {
		// Handle error (optional: log or return an empty list of threads)
		fmt.Printf("Error parsing goroutines: %v\n", err)
		return nil, []byte("")
	}

	// Convert parsed goroutines to Sentry.Thread format
	var threads []sentry.Thread

	for _, g := range goroutines {
		thread := convertGoroutineToThread(g)
		threads = append(threads, thread)
	}

	// Return the list of Sentry threads and also the raw stacktrace string for additional logging or debugging
	return threads, stack
}

func entireStack() []byte {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return buf[:n]
		}
		buf = make([]byte, 2*len(buf))
	}
}

// convertGoroutineToThread converts a parsed Goroutine to a Sentry Thread object
func convertGoroutineToThread(g *gostackparse.Goroutine) sentry.Thread {
	// Convert each Goroutine's stack frames to Sentry frames
	frames := convertFrames(g.Stack)

	// Create a Sentry stacktrace
	stacktrace := &sentry.Stacktrace{
		Frames: frames,
	}

	// Create a Sentry thread
	return sentry.Thread{
		ID:         fmt.Sprintf("%d", g.ID),
		Name:       fmt.Sprintf("Goroutine %d", g.ID),
		Stacktrace: stacktrace,
		Crashed:    false, // Adjust based on actual crash status if needed
		Current:    false, // You can refine this if you track the "current" thread
	}
}

// convertFrames converts a slice of gostackparse.Frame to a slice of sentry.Frame
func convertFrames(goroutineFrames []*gostackparse.Frame) []sentry.Frame {
	var frames []sentry.Frame
	for _, gf := range goroutineFrames {
		absPath := gf.File
		fileName := filepath.Base(absPath)
		frame := sentry.Frame{
			Function: gf.Func,
			Filename: fileName,
			Lineno:   gf.Line,
			AbsPath:  absPath,
		}
		frames = append(frames, frame)
	}
	return frames
}
