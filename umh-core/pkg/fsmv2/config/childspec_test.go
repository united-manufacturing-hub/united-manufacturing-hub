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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

var _ = Describe("ChildSpec", func() {
	Describe("YAML serialization", func() {
		It("should serialize to YAML correctly", func() {
			spec := config.ChildSpec{
				Name:       "connection",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "mqtt:\n  url: tcp://localhost:1883",
				},
			}

			data, err := yaml.Marshal(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("name: connection"))
			Expect(string(data)).To(ContainSubstring("workerType: mqtt_connection"))
		})

		It("should deserialize from YAML correctly", func() {
			yamlData := `
name: connection
workerType: mqtt_connection
userSpec:
  config: "mqtt:\n  url: tcp://localhost:1883"
`
			var spec config.ChildSpec
			err := yaml.Unmarshal([]byte(yamlData), &spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(spec.Name).To(Equal("connection"))
			Expect(spec.WorkerType).To(Equal("mqtt_connection"))
			Expect(spec.UserSpec.Config).To(ContainSubstring("tcp://localhost:1883"))
		})
	})

	Describe("JSON serialization", func() {
		It("should serialize to JSON correctly", func() {
			spec := config.ChildSpec{
				Name:       "dataflow",
				WorkerType: "kafka_consumer",
				UserSpec: config.UserSpec{
					Config: "topic: sensor-data",
				},
			}

			data, err := json.Marshal(spec)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring(`"name":"dataflow"`))
			Expect(string(data)).To(ContainSubstring(`"workerType":"kafka_consumer"`))
		})
	})
})

var _ = Describe("ChildSpec Clone", func() {
	Describe("Clone()", func() {
		It("should create a deep copy that is independent from the original", func() {
			original := config.ChildSpec{
				Name:       "original-worker",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "original-config",
					Variables: config.VariableBundle{
						User: map[string]interface{}{
							"key1": "value1",
						},
					},
				},
			}

			cloned := original.Clone()

			Expect(cloned.Name).To(Equal(original.Name))
			Expect(cloned.WorkerType).To(Equal(original.WorkerType))
			Expect(cloned.UserSpec.Config).To(Equal(original.UserSpec.Config))
		})
	})
})

var _ = Describe("ChildSpec Hash", func() {
	Describe("Hash()", func() {
		It("should produce deterministic hashes for identical specs", func() {
			spec1 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "test-config",
					Variables: config.VariableBundle{
						User: map[string]interface{}{
							"key": "value",
						},
					},
				},
			}

			spec2 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec: config.UserSpec{
					Config: "test-config",
					Variables: config.VariableBundle{
						User: map[string]interface{}{
							"key": "value",
						},
					},
				},
			}

			hash1, err1 := spec1.Hash()
			Expect(err1).NotTo(HaveOccurred())
			hash2, err2 := spec2.Hash()
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).To(Equal(hash2))
		})

		It("should produce different hashes when Name changes", func() {
			spec1 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			spec2 := config.ChildSpec{
				Name:       "child-2",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			hash1, err1 := spec1.Hash()
			Expect(err1).NotTo(HaveOccurred())
			hash2, err2 := spec2.Hash()
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should produce different hashes when WorkerType changes", func() {
			spec1 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			spec2 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "modbus_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			hash1, err1 := spec1.Hash()
			Expect(err1).NotTo(HaveOccurred())
			hash2, err2 := spec2.Hash()
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should produce different hashes when UserSpec.Config changes", func() {
			spec1 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config-a"},
			}

			spec2 := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config-b"},
			}

			hash1, err1 := spec1.Hash()
			Expect(err1).NotTo(HaveOccurred())
			hash2, err2 := spec2.Hash()
			Expect(err2).NotTo(HaveOccurred())
			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should produce a 16-character hex string", func() {
			spec := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
			}

			hash, err := spec.Hash()
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(HaveLen(16))
			// All characters should be hex digits
			for _, c := range hash {
				Expect(string(c)).To(MatchRegexp("[0-9a-f]"))
			}
		})

		It("should produce different hashes when Enabled flips", func() {
			specEnabled := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
				Enabled:    true,
			}

			specDisabled := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
				Enabled:    false,
			}

			hashEnabled, errA := specEnabled.Hash()
			Expect(errA).NotTo(HaveOccurred())
			hashDisabled, errB := specDisabled.Hash()
			Expect(errB).NotTo(HaveOccurred())
			Expect(hashEnabled).NotTo(Equal(hashDisabled),
				"Enabled flip must change the hash so dependent caches re-validate the child spec")
		})

		It("Clone preserves Enabled in the hash", func() {
			spec := config.ChildSpec{
				Name:       "child-1",
				WorkerType: "mqtt_connection",
				UserSpec:   config.UserSpec{Config: "config"},
				Enabled:    true,
			}

			origHash, errOrig := spec.Hash()
			Expect(errOrig).NotTo(HaveOccurred())
			cloneHash, errClone := spec.Clone().Hash()
			Expect(errClone).NotTo(HaveOccurred())
			Expect(cloneHash).To(Equal(origHash),
				"Clone copies Enabled (value type), so the cloned hash matches the original")
		})
	})
})

var _ = Describe("DesiredState", func() {
	Describe("YAML serialization", func() {
		It("should serialize to YAML correctly", func() {
			desired := config.DesiredState{
				BaseDesiredState: config.BaseDesiredState{State: "running"},
				ChildrenSpecs: []config.ChildSpec{
					{
						Name:       "child1",
						WorkerType: "worker_type_a",
						UserSpec: config.UserSpec{
							Config: "config1",
						},
					},
				},
			}

			data, err := yaml.Marshal(desired)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("state: running"))
			Expect(string(data)).To(ContainSubstring("childrenSpecs:"))
			Expect(string(data)).To(ContainSubstring("name: child1"))
		})

		It("should deserialize from YAML correctly", func() {
			yamlData := `
state: active
childrenSpecs:
  - name: connection
    workerType: mqtt
    userSpec:
      config: "mqtt config"
  - name: dataflow
    workerType: processor
    userSpec:
      config: "processor config"
`
			var desired config.DesiredState
			err := yaml.Unmarshal([]byte(yamlData), &desired)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("active"))
			Expect(desired.ChildrenSpecs).To(HaveLen(2))
			Expect(desired.ChildrenSpecs[0].Name).To(Equal("connection"))
			Expect(desired.ChildrenSpecs[1].Name).To(Equal("dataflow"))
		})

		It("should handle empty ChildrenSpecs correctly", func() {
			yamlData := `
state: stopped
`
			var desired config.DesiredState
			err := yaml.Unmarshal([]byte(yamlData), &desired)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired.State).To(Equal("stopped"))
			Expect(desired.ChildrenSpecs).To(BeEmpty())
		})
	})

	Describe("ShutdownRequested interface", func() {
		It("should return false when State is not 'shutdown'", func() {
			desired := config.DesiredState{
				BaseDesiredState: config.BaseDesiredState{State: "running"},
			}

			Expect(desired.IsShutdownRequested()).To(BeFalse())
		})

		It("should return true when ShutdownRequested is set", func() {
			desired := config.DesiredState{}
			desired.SetShutdownRequested(true)

			Expect(desired.IsShutdownRequested()).To(BeTrue())
		})
	})
})

var _ = Describe("ChildInfo JSON dual-read", func() {
	It("unmarshals legacy PascalCase JSON into camelCase fields", func() {
		legacy := []byte(`{
			"Name": "child-1",
			"WorkerType": "mqtt_client",
			"StateName": "running_healthy_connected",
			"StateReason": "all good",
			"ErrorMsg": "",
			"HierarchyPath": "app.parent.child-1",
			"IsHealthy": true,
			"IsStale": false,
			"IsCircuitOpen": false
		}`)

		var info config.ChildInfo
		Expect(json.Unmarshal(legacy, &info)).To(Succeed())

		Expect(info.Name).To(Equal("child-1"))
		Expect(info.WorkerType).To(Equal("mqtt_client"))
		Expect(info.StateName).To(Equal("running_healthy_connected"))
		Expect(info.StateReason).To(Equal("all good"))
		Expect(info.HierarchyPath).To(Equal("app.parent.child-1"))
		Expect(info.IsHealthy).To(BeTrue())
	})

	It("unmarshals new camelCase JSON", func() {
		modern := []byte(`{
			"name": "child-2",
			"workerType": "modbus_client",
			"stateName": "trying_to_start_dialing",
			"stateReason": "waiting for handshake",
			"errorMsg": "timeout",
			"hierarchyPath": "app.parent.child-2",
			"isHealthy": false,
			"isStale": true,
			"isCircuitOpen": true
		}`)

		var info config.ChildInfo
		Expect(json.Unmarshal(modern, &info)).To(Succeed())

		Expect(info.Name).To(Equal("child-2"))
		Expect(info.WorkerType).To(Equal("modbus_client"))
		Expect(info.StateName).To(Equal("trying_to_start_dialing"))
		Expect(info.ErrorMsg).To(Equal("timeout"))
		Expect(info.IsStale).To(BeTrue())
		Expect(info.IsCircuitOpen).To(BeTrue())
	})

	It("round-trips legacy PascalCase JSON to camelCase JSON without losing data", func() {
		legacy := []byte(`{
			"Name": "child-3",
			"WorkerType": "opcua_client",
			"StateName": "running_degraded_polling",
			"StateReason": "partial connection",
			"ErrorMsg": "intermittent",
			"HierarchyPath": "app.parent.child-3",
			"IsHealthy": false,
			"IsStale": false,
			"IsCircuitOpen": false
		}`)

		var info config.ChildInfo
		Expect(json.Unmarshal(legacy, &info)).To(Succeed())

		out, err := json.Marshal(info)
		Expect(err).ToNot(HaveOccurred())

		// Re-emitted JSON uses the new camelCase form.
		Expect(string(out)).To(ContainSubstring(`"name":"child-3"`))
		Expect(string(out)).To(ContainSubstring(`"workerType":"opcua_client"`))
		Expect(string(out)).To(ContainSubstring(`"stateName":"running_degraded_polling"`))

		// PascalCase tags must not reappear: decode the JSON and check that no
		// top-level PascalCase keys exist. Substring scans are brittle because
		// the substring "Name" can occur inside other tag names like "stateName".
		var keyed map[string]json.RawMessage
		Expect(json.Unmarshal(out, &keyed)).To(Succeed())

		for _, legacyKey := range []string{
			"Name", "WorkerType", "StateName", "StateReason",
			"ErrorMsg", "HierarchyPath", "IsHealthy", "IsStale", "IsCircuitOpen",
		} {
			Expect(keyed).NotTo(HaveKey(legacyKey),
				"legacy PascalCase tag %q must not appear in re-emitted JSON", legacyKey)
		}

		// Re-decoded data matches the original logical content.
		var roundTrip config.ChildInfo
		Expect(json.Unmarshal(out, &roundTrip)).To(Succeed())
		Expect(roundTrip).To(Equal(info))
	})

	It("merges mixed legacy + camelCase fields without losing either", func() {
		mixed := []byte(`{
			"name": "child-mixed",
			"WorkerType": "kafka_consumer",
			"stateName": "Connected",
			"phase": 3,
			"IsHealthy": true,
			"isStale": true
		}`)

		var info config.ChildInfo
		Expect(json.Unmarshal(mixed, &info)).To(Succeed())

		Expect(info.Name).To(Equal("child-mixed"), "camelCase 'name' must win when present")
		Expect(info.WorkerType).To(Equal("kafka_consumer"),
			"PascalCase 'WorkerType' must fill in when camelCase omits the field")
		Expect(info.StateName).To(Equal("Connected"))
		Expect(info.Phase).To(Equal(config.PhaseRunningHealthy),
			"camelCase 'phase' decodes into the cached LifecyclePhase")
		Expect(info.IsHealthy).To(BeTrue(),
			"legacy 'IsHealthy: true' must not be lost when camelCase form is absent")
		Expect(info.IsStale).To(BeTrue())
	})

	It("camelCase bool 'false' does NOT override legacy bool 'true' (locks asymmetric merge)", func() {
		// Documents the LOCKED design: the merge is asymmetric on purpose.
		// camelCase wins when its value is non-zero; the zero value (false) is
		// indistinguishable from "field absent" so the legacy form fills in.
		// A future maintainer who "fixes" this asymmetry must understand the
		// tradeoff before changing UnmarshalJSON: legacy writers emitting
		// IsHealthy=true must not be silently downgraded to false by newer
		// readers that omit the camelCase field.
		raw := []byte(`{"isHealthy": false, "IsHealthy": true, "isStale": false, "IsStale": true, "isCircuitOpen": false, "IsCircuitOpen": true}`)

		var info config.ChildInfo
		Expect(json.Unmarshal(raw, &info)).To(Succeed())
		Expect(info.IsHealthy).To(BeTrue(),
			"camelCase false must NOT override legacy true — zero-value-fallback is the LOCKED contract")
		Expect(info.IsStale).To(BeTrue(),
			"same precedence rule applies to IsStale")
		Expect(info.IsCircuitOpen).To(BeTrue(),
			"same precedence rule applies to IsCircuitOpen")
	})
})

var _ = Describe("ChildrenView aggregate predicates (LOCKED §4-B)", func() {
	It("empty children slice yields all-true predicates and zero counts (locks short-circuit)", func() {
		v := config.NewChildrenView(nil)
		Expect(v.AllHealthy).To(BeTrue())
		Expect(v.AllOperational).To(BeTrue())
		Expect(v.AllStopped).To(BeTrue())
		Expect(v.HealthyCount).To(Equal(0))
		Expect(v.UnhealthyCount).To(Equal(0))
	})

	It("classifies a single PhaseRunningHealthy child", func() {
		v := config.NewChildrenView([]config.ChildInfo{
			{Name: "a", StateName: "Connected", Phase: config.PhaseRunningHealthy, IsHealthy: true},
		})
		Expect(v.HealthyCount).To(Equal(1))
		Expect(v.UnhealthyCount).To(Equal(0))
		Expect(v.AllHealthy).To(BeTrue())
		Expect(v.AllOperational).To(BeTrue())
		Expect(v.AllStopped).To(BeFalse())
	})

	It("classifies a child by Phase even when StateName is a raw worker state name", func() {
		// Regression guard for the ParseLifecyclePhase(StateName) bug:
		// production children carry raw state names like "Connected" that the
		// prefix-based parser cannot classify, so predicates must read the
		// cached Phase field that the supervisor populates.
		v := config.NewChildrenView([]config.ChildInfo{
			{Name: "a", StateName: "Connected", Phase: config.PhaseRunningHealthy, IsHealthy: true},
		})
		Expect(v.HealthyCount).To(Equal(1), "Phase field must drive classification, not StateName parsing")
		Expect(v.AllHealthy).To(BeTrue())
	})

	It("treats Healthy + Degraded children as Operational but not AllHealthy", func() {
		v := config.NewChildrenView([]config.ChildInfo{
			{Name: "a", StateName: "Connected", Phase: config.PhaseRunningHealthy, IsHealthy: true},
			{Name: "b", StateName: "Reconnecting", Phase: config.PhaseRunningDegraded, IsHealthy: false},
		})
		Expect(v.HealthyCount).To(Equal(1))
		Expect(v.UnhealthyCount).To(Equal(1),
			"PhaseRunningDegraded contributes to UnhealthyCount per ChildrenManager.Counts semantics")
		Expect(v.AllHealthy).To(BeFalse())
		Expect(v.AllOperational).To(BeTrue(), "Healthy + Degraded both qualify as Operational")
		Expect(v.AllStopped).To(BeFalse())
	})

	It("classifies a single PhaseStopped child as AllStopped (and neither healthy nor unhealthy)", func() {
		v := config.NewChildrenView([]config.ChildInfo{
			{Name: "a", StateName: "Stopped", Phase: config.PhaseStopped, IsHealthy: false},
		})
		Expect(v.HealthyCount).To(Equal(0))
		Expect(v.UnhealthyCount).To(Equal(0),
			"PhaseStopped is neutral: contributes to neither healthy nor unhealthy count")
		Expect(v.AllHealthy).To(BeFalse())
		Expect(v.AllOperational).To(BeFalse())
		Expect(v.AllStopped).To(BeTrue())
	})

	It("treats PhaseUnknown as unhealthy (locks iota-zero classification)", func() {
		// Phase is the iota zero-value, so legacy snapshots that omit "phase"
		// (and any future ChildInfo decoded with Phase explicitly unset)
		// arrive as PhaseUnknown. The aggregate predicates must classify
		// PhaseUnknown as unhealthy — it is not Running*, not Stopped, not
		// Operational. This spec also locks the iota ordering itself: a future
		// reorder that swaps PhaseUnknown with PhaseStopped would silently
		// flip these aggregates (legacy unknown children counted as Stopped),
		// and this test would catch the swap.
		v := config.NewChildrenView([]config.ChildInfo{
			{Name: "a", StateName: "", Phase: config.PhaseUnknown, IsHealthy: false},
		})
		Expect(v.HealthyCount).To(Equal(0),
			"PhaseUnknown does not contribute to HealthyCount")
		Expect(v.UnhealthyCount).To(Equal(1),
			"PhaseUnknown counts as unhealthy — neither Running* nor Stopped")
		Expect(v.AllHealthy).To(BeFalse())
		Expect(v.AllOperational).To(BeFalse(),
			"PhaseUnknown is not Operational")
		Expect(v.AllStopped).To(BeFalse(),
			"PhaseUnknown is not Stopped — locks iota ordering against accidental reorder")
	})
})

var _ = Describe("ChildrenView nil-children normalisation", func() {
	It("emits children:[] (not children:null) so CSE delta-sync is stable", func() {
		v := config.NewChildrenView(nil)
		out, err := json.Marshal(v)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(out)).To(ContainSubstring(`"children":[]`),
			"nil children slice must serialise as an empty JSON array, not null")
		Expect(string(out)).NotTo(ContainSubstring(`"children":null`))
	})

	It("preserves an explicitly empty slice through serialisation", func() {
		v := config.NewChildrenView([]config.ChildInfo{})
		out, err := json.Marshal(v)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(out)).To(ContainSubstring(`"children":[]`))
	})
})
