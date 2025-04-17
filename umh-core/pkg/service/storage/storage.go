// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package storage provides a storage service to store the state transition histories or events for each FSM
package storage

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Record is the record that is stored in the archive
// It contains the state that the FSM is in and the event that caused the state transition
type Record struct {
	ID          string // The ID of the FSM
	State       string // The state that the FSM is in
	SourceEvent string // The event that caused the state transition
}

// DataPoint is the individual data point that is stored in the archive
// Its abstraction is very similar to a time series database's data point
type DataPoint struct {
	Record Record    // The record that is stored
	Time   time.Time // The time that the event occurred
}

// ArchiveStorer is the interface for the archive storage service
// This is intended to be used only in the FSMs themselves
type ArchiveStorer interface {
	StoreDataPoint(ctx context.Context, dataPoint DataPoint) error
	GetDataPoints(ctx context.Context, id string, options QueryOptions) ([]DataPoint, error)
}

// ArchiveStoreCloser is the interface for the archive storage service that also includes a Close method
// This is intended to be used only in the supervising control loop, not in the FSMs themselves
type ArchiveStoreCloser interface {
	ArchiveStorer
	Close() error
}

// QueryOptions provides filtering options for retrieving data points
type QueryOptions struct {
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	States    []string
	SortDesc  bool
}

var _ ArchiveStoreCloser = &ArchiveEventStorage{}

// ArchiveEventStorage is the implementation of the ArchiveStorer interface
type ArchiveEventStorage struct {
	dataPoints        map[string][]DataPoint
	dataPointQueue    chan DataPoint
	done              chan struct{}
	mu                sync.RWMutex
	maxPointsPerState int
}

// NewArchiveEventStorage creates a new ArchiveEventStorage
func NewArchiveEventStorage(maxPointsPerState int) *ArchiveEventStorage {
	if maxPointsPerState <= 0 {
		maxPointsPerState = 1000 // Default limit
	}

	a := &ArchiveEventStorage{
		dataPoints:        make(map[string][]DataPoint),
		dataPointQueue:    make(chan DataPoint, 100),
		done:              make(chan struct{}),
		maxPointsPerState: maxPointsPerState,
	}

	go a.storeDataPoints()
	return a
}

func (a *ArchiveEventStorage) storeDataPoints() {
	for {
		select {
		case dataPoint := <-a.dataPointQueue:
			a.mu.Lock()
			points := a.dataPoints[dataPoint.Record.ID]
			// Enforce max points limit per state
			if len(points) >= a.maxPointsPerState {
				// Remove oldest point
				points = points[1:]
			}
			points = append(points, dataPoint)
			a.dataPoints[dataPoint.Record.ID] = points
			a.mu.Unlock()
		case <-a.done:
			return
		}
	}
}

func (a *ArchiveEventStorage) StoreDataPoint(ctx context.Context, dataPoint DataPoint) error {
	select {
	case a.dataPointQueue <- dataPoint:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Queue is full
		return errors.New("storage queue is full, try again later")
	}
}

func (a *ArchiveEventStorage) GetDataPoints(ctx context.Context, id string, options QueryOptions) ([]DataPoint, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		points := a.dataPoints[id]
		if points == nil {
			return []DataPoint{}, nil
		}

		// Filter by time range
		var filtered []DataPoint
		for _, p := range points {
			if (!options.StartTime.IsZero() && p.Time.Before(options.StartTime)) ||
				(!options.EndTime.IsZero() && p.Time.After(options.EndTime)) {
				continue
			}
			filtered = append(filtered, p)
		}

		// Filter by states
		if len(options.States) > 0 {
			filtered = filterByStates(filtered, options.States)
		}

		// Sort if needed
		if options.SortDesc {
			sort.Slice(filtered, func(i, j int) bool {
				return filtered[i].Time.After(filtered[j].Time)
			})
		}

		// Apply limit
		if options.Limit > 0 && len(filtered) > options.Limit {
			filtered = filtered[:options.Limit]
		}

		return filtered, nil
	}
}

// filterByStates filters data points based on the provided states
func filterByStates(dataPoints []DataPoint, states []string) []DataPoint {
	if len(states) == 0 {
		return dataPoints
	}

	// Create a map for faster lookups
	stateMap := make(map[string]struct{}, len(states))
	for _, state := range states {
		stateMap[state] = struct{}{}
	}

	filtered := make([]DataPoint, 0, len(dataPoints))
	for _, dp := range dataPoints {
		if _, ok := stateMap[dp.Record.State]; ok {
			filtered = append(filtered, dp)
		}
	}

	return filtered
}

func (a *ArchiveEventStorage) Close() error {
	close(a.done)
	return nil
}
