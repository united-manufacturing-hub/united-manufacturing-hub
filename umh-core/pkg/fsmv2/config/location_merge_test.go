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

package config_test

import (
	"errors"
	"maps"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("MergeAgentLocation", func() {
	mergeCases := []struct {
		name       string
		agent      map[string]string
		own        map[string]string
		wantMerged map[string]string
		wantPath   string
	}{
		{
			name:       "own levels fill an empty agent level, extend the map, and gaps become unknown",
			agent:      map[string]string{"0": "acme", "1": ""},
			own:        map[string]string{"1": "hall", "3": "cell"},
			wantMerged: map[string]string{"0": "acme", "1": "hall", "2": "unknown", "3": "cell"},
			wantPath:   "acme.hall.unknown.cell",
		},
		{
			name:       "the agent value is kept on a shared level",
			agent:      map[string]string{"0": "acme", "1": "plant"},
			own:        map[string]string{"1": "other"},
			wantMerged: map[string]string{"0": "acme", "1": "plant"},
			wantPath:   "acme.plant",
		},
		{
			name:       "an empty agent level that no own level fills stays empty and the path drops its trailing dot",
			agent:      map[string]string{"0": "a", "1": ""},
			own:        nil,
			wantMerged: map[string]string{"0": "a", "1": ""},
			wantPath:   "a",
		},
		{
			name:       "an empty level that both sides define stays empty",
			agent:      map[string]string{"0": "a", "1": ""},
			own:        map[string]string{"1": ""},
			wantMerged: map[string]string{"0": "a", "1": ""},
			wantPath:   "a",
		},
		{
			name:       "an empty level between two defined levels stays empty in the path",
			agent:      map[string]string{"0": "a", "1": "", "2": "c"},
			own:        nil,
			wantMerged: map[string]string{"0": "a", "1": "", "2": "c"},
			wantPath:   "a..c",
		},
		{
			name:       "a nil agent location leaves only the worker's own levels",
			agent:      nil,
			own:        map[string]string{"0": "a"},
			wantMerged: map[string]string{"0": "a"},
			wantPath:   "a",
		},
		{
			name:       "both empty gives an empty map and an empty path",
			agent:      nil,
			own:        nil,
			wantMerged: map[string]string{},
			wantPath:   "",
		},
		{
			name:       "a level key above the cap is copied but does not drive the fill loop",
			agent:      map[string]string{},
			own:        map[string]string{"2000000000": "x"},
			wantMerged: map[string]string{"2000000000": "x"},
			wantPath:   "",
		},
		{
			name:       "a key that is not a level index is copied but never affects the path",
			agent:      map[string]string{"enterprise": "acme"},
			own:        nil,
			wantMerged: map[string]string{"enterprise": "acme"},
			wantPath:   "",
		},
		{
			name:       "a non-canonical integer key is copied but never affects the path",
			agent:      map[string]string{"0": "acme"},
			own:        map[string]string{"01": "x", "+2": "y"},
			wantMerged: map[string]string{"0": "acme", "01": "x", "+2": "y"},
			wantPath:   "acme",
		},
	}

	for _, tc := range mergeCases {
		Context(tc.name, func() {
			It("returns the merged map and path without modifying the inputs", func() {
				agentBefore := maps.Clone(tc.agent)
				ownBefore := maps.Clone(tc.own)

				merged, path := config.MergeAgentLocation(tc.agent, tc.own)

				Expect(merged).To(Equal(tc.wantMerged))
				Expect(path).To(Equal(tc.wantPath))
				Expect(tc.agent).To(Equal(agentBefore))
				Expect(tc.own).To(Equal(ownBefore))
			})
		})
	}
})

var _ = Describe("LocationFromAny", func() {
	Describe("accepted inputs", func() {
		It("accepts a map with yaml integer keys", func() {
			got, err := config.LocationFromAny(map[any]any{1: "hall", 3: "cell"})
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(map[string]string{"1": "hall", "3": "cell"}))
		})

		It("accepts a map with canonical string keys", func() {
			got, err := config.LocationFromAny(map[any]any{"0": "hall", "2": "cell"})
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(map[string]string{"0": "hall", "2": "cell"}))
		})

		It("accepts map[string]any with a number value", func() {
			got, err := config.LocationFromAny(map[string]any{"0": "hall", "1": 2})
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(map[string]string{"0": "hall", "1": "2"}))
		})

		It("accepts map[string]string", func() {
			got, err := config.LocationFromAny(map[string]string{"1": "hall"})
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(map[string]string{"1": "hall"}))
		})

		It("returns an empty map for nil", func() {
			got, err := config.LocationFromAny(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(BeEmpty())
		})

		It("accepts a level at the cap and rejects the one above it", func() {
			atCap, err := config.LocationFromAny(map[any]any{1000: "a"})
			Expect(err).NotTo(HaveOccurred())
			Expect(atCap).To(Equal(map[string]string{"1000": "a"}))

			_, err = config.LocationFromAny(map[any]any{1001: "a"})
			expectLocationError(err)
		})
	})

	Describe("rejected inputs", func() {
		It("rejects a non-scalar value with a config error naming the level", func() {
			_, err := config.LocationFromAny(map[any]any{"0": []any{}})
			expectLocationError(err)
			Expect(err.Error()).To(ContainSubstring(`level "0"`))
		})

		It("rejects a value that is neither a string nor a number", func() {
			_, err := config.LocationFromAny(map[any]any{"0": true})
			expectLocationError(err)
		})

		It("rejects input that is not a mapping", func() {
			_, err := config.LocationFromAny("plant")
			expectLocationError(err)
		})

		invalidKeys := []struct {
			name string
			raw  map[any]any
		}{
			{name: "a named key", raw: map[any]any{"enterprise": "acme"}},
			{name: "a negative string key", raw: map[any]any{"-1": "a"}},
			{name: "a negative integer key", raw: map[any]any{-1: "a"}},
			{name: "a zero-padded key", raw: map[any]any{"02": "a"}},
			{name: "a plus-prefixed key", raw: map[any]any{"+1": "a"}},
			{name: "two keys spelling the same level", raw: map[any]any{1: "a", "1": "b"}},
			{name: "a non-integer non-string key", raw: map[any]any{1.5: "a"}},
		}

		for _, tc := range invalidKeys {
			It("rejects "+tc.name, func() {
				_, err := config.LocationFromAny(tc.raw)
				expectLocationError(err)
			})
		}
	})
})

func expectLocationError(err error) {
	ExpectWithOffset(1, err).To(HaveOccurred())
	var cfgErr *config.ConfigError
	ExpectWithOffset(1, errors.As(err, &cfgErr) && cfgErr.Field == "location").To(BeTrue(),
		"want a *config.ConfigError for field location, got %v", err)
}
