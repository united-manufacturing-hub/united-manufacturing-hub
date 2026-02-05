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

package deps

import "time"

// Scheduler provides time-based scheduling decisions for FSMv2 workers.
// Methods are timezone-agnostic — they operate on the timezone of the
// provided time.Time. Containers typically run in UTC, so time.Now()
// produces UTC. The transport worker uses time.Now().Local() explicitly.
// Both approaches work — the caller decides.
//
// Maintenance window hierarchy (subset relationship):
//   - IsPreferredMaintenanceWindow ⊂ IsAcceptableMaintenanceWindow
//   - Preferred = weekend AND low-traffic (best time for heavy ops like VACUUM)
//   - Acceptable = weekend OR low-traffic (good-enough fallback after target interval)
type Scheduler interface {
	IsWeekend(t time.Time) bool
	IsLowTrafficHours(t time.Time) bool
	IsBusinessHours(t time.Time) bool
	IsPreferredMaintenanceWindow(t time.Time) bool
	IsAcceptableMaintenanceWindow(t time.Time) bool
}

// DefaultScheduler is the production Scheduler implementation.
//
// Hour windows:
//   - Low-traffic: 02:00–05:00
//   - Business:    07:00–20:00
//
// Maintenance windows:
//   - Preferred: weekend AND low-traffic hours (best time for heavy ops)
//   - Acceptable: weekend OR low-traffic hours (good-enough fallback)
type DefaultScheduler struct{}

var _ Scheduler = DefaultScheduler{}

func (DefaultScheduler) IsWeekend(t time.Time) bool {
	d := t.Weekday()
	return d == time.Saturday || d == time.Sunday
}

func (DefaultScheduler) IsLowTrafficHours(t time.Time) bool {
	h := t.Hour()
	return h >= 2 && h < 5
}

func (DefaultScheduler) IsBusinessHours(t time.Time) bool {
	h := t.Hour()
	return h >= 7 && h < 20
}

func (DefaultScheduler) IsPreferredMaintenanceWindow(t time.Time) bool {
	d := t.Weekday()
	h := t.Hour()
	return (d == time.Saturday || d == time.Sunday) && h >= 2 && h < 5
}

func (DefaultScheduler) IsAcceptableMaintenanceWindow(t time.Time) bool {
	d := t.Weekday()
	h := t.Hour()
	return (d == time.Saturday || d == time.Sunday) || (h >= 2 && h < 5)
}
