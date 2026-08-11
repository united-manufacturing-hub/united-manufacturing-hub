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

package config

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

var _ = Describe("data contract lifecycle", func() {
	var (
		mockFS        *filesystem.MockFileSystem
		configManager *FileConfigManager
		ctx           context.Context
		writtenData   []byte
	)

	// onDisk points the manager at a config and captures whatever it writes back.
	onDisk := func(yaml string) {
		writtenData = nil
		current := []byte(yaml)

		mockFS.WithEnsureDirectoryFunc(func(ctx context.Context, path string) error { return nil })
		mockFS.WithFileExistsFunc(func(ctx context.Context, path string) (bool, error) {
			return true, nil
		})
		mockFS.WithReadFileFunc(func(ctx context.Context, path string) ([]byte, error) {
			return current, nil
		})
		mockFS.WithWriteFileFunc(func(ctx context.Context, path string, data []byte, perm os.FileMode) error {
			writtenData = data

			return nil
		})
		mockFS.WithStatFunc(func(ctx context.Context, path string) (os.FileInfo, error) {
			return mockFS.NewMockFileInfo(
				"config.yaml", int64(len(writtenData)), 0644, time.Now(), false), nil
		})
	}

	// written re-parses what was written, which is the only thing that proves an
	// operation persisted rather than merely returning nil.
	written := func() FullConfig {
		Expect(writtenData).NotTo(BeEmpty(), "nothing was written")

		config, err := ParseConfig(writtenData, ctx, false)
		Expect(err).NotTo(HaveOccurred(), "what we wrote must parse")

		return config
	}

	contractNamed := func(config FullConfig, name string) *DataContract {
		for i := range config.Contracts {
			if config.Contracts[i].Name == name {
				return &config.Contracts[i]
			}
		}

		return nil
	}

	const bareConfig = `
agent:
  metricsPort: 8080
`

	const pumpConfig = `
agent:
  metricsPort: 8080
dataContracts:
  - model: pump
    description: Pump monitoring
    versions:
      v1:
        name: _pump_v1
        structure:
          temperature:
            _payloadshape: timeseries-number
`

	BeforeEach(func() {
		mockFS = filesystem.NewMockFileSystem()
		ctx = context.Background()
	})

	JustBeforeEach(func() {
		configManager = NewFileConfigManager()
		configManager.WithFileSystemService(mockFS)
	})

	AfterEach(func() {
		configManager.Stop()
	})

	Describe("adding a contract", func() {
		It("creates the group and its v1 address in a single write", func() {
			onDisk(bareConfig)

			structure := map[string]Field{"temperature": {PayloadShape: "timeseries-number"}}

			Eventually(func() error {
				return configManager.AtomicAddDataContract(ctx, "temperature", structure, "a description")
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

			config := written()
			Expect(config.Contracts).To(HaveLen(1))
			Expect(config.Contracts[0].Name).To(Equal("_temperature_v1"))
			Expect(config.Contracts[0].Model).To(Equal("temperature"))
			Expect(config.Contracts[0].Version).To(Equal("v1"))
			Expect(config.Contracts[0].Description).To(Equal("a description"))
			Expect(config.Contracts[0].Structure).To(HaveKey("temperature"))

			// The migration happened too: one section, not two.
			Expect(config.DataModels).To(BeEmpty())
		})

		It("refuses a label that already exists", func() {
			onDisk(pumpConfig)

			Eventually(func() error {
				return configManager.AtomicAddDataContract(ctx, "pump", nil, "")
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(
				MatchError(ContainSubstring("already exists")))
		})

		It("produces a contract set that survives the round trip", func() {
			onDisk(bareConfig)

			structure := map[string]Field{"t": {PayloadShape: "timeseries-number"}}

			Eventually(func() error {
				return configManager.AtomicAddDataContract(ctx, "pump", structure, "d")
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

			// Every operation's output has to be expressible in both shapes, or the next
			// boot degrades and refuses further changes.
			Expect(ContractsAreLossless(written().Contracts)).To(BeTrue())
		})
	})

	Describe("adding a version", func() {
		It("mints the next version and reports its address", func() {
			onDisk(pumpConfig)

			var address, version string

			Eventually(func() error {
				var err error
				address, version, err = configManager.AtomicAddDataContractVersion(
					ctx, "pump", map[string]Field{"pressure": {PayloadShape: "timeseries-number"}},
					"Pump monitoring v2")

				return err
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

			Expect(version).To(Equal("v2"))
			Expect(address).To(Equal("_pump_v2"))

			config := written()
			Expect(config.Contracts).To(HaveLen(2))

			// v1 keeps its address and its structure: versions are append-only, because
			// editing a released one changes what an address already in use enforces.
			v1 := contractNamed(config, "_pump_v1")
			Expect(v1).NotTo(BeNil())
			Expect(v1.Structure).To(HaveKey("temperature"))
			Expect(v1.Structure).NotTo(HaveKey("pressure"))

			v2 := contractNamed(config, "_pump_v2")
			Expect(v2).NotTo(BeNil())
			Expect(v2.Structure).To(HaveKey("pressure"))
		})

		It("restates the description across the whole group", func() {
			onDisk(pumpConfig)

			Eventually(func() error {
				_, _, err := configManager.AtomicAddDataContractVersion(
					ctx, "pump", map[string]Field{"p": {PayloadShape: "timeseries-number"}},
					"a new description")

				return err
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

			// The description belongs to the group, so leaving old entries with the old
			// one would make the emitted YAML depend on write order.
			for _, contract := range written().Contracts {
				Expect(contract.Description).To(Equal("a new description"))
			}
		})

		It("refuses a label that does not exist", func() {
			onDisk(pumpConfig)

			Eventually(func() error {
				_, _, err := configManager.AtomicAddDataContractVersion(ctx, "absent", nil, "")

				return err
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(MatchError(ContainSubstring("not found")))
		})
	})

	Describe("deleting one address", func() {
		It("leaves the structure behind as a definition", func() {
			onDisk(pumpConfig)

			Eventually(func() error {
				return configManager.AtomicDeleteDataContract(ctx, "_pump_v1")
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

			config := written()

			// The version survives with no address, so anything pointing at pump v1
			// through _refModel keeps resolving and its subjects keep validating.
			Expect(config.Contracts).To(HaveLen(1))
			Expect(config.Contracts[0].Name).To(BeEmpty())
			Expect(config.Contracts[0].Model).To(Equal("pump"))
			Expect(config.Contracts[0].Version).To(Equal("v1"))
			Expect(config.Contracts[0].Structure).To(HaveKey("temperature"))
		})

		It("removes a bare address outright, since it carries no structure", func() {
			onDisk(`
agent:
  metricsPort: 8080
dataContracts:
  - name: _raw
`)

			Eventually(func() error {
				return configManager.AtomicDeleteDataContract(ctx, "_raw")
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

			Expect(written().Contracts).To(BeEmpty())
		})

		It("refuses while a bridge still publishes to the address", func() {
			onDisk(`
agent:
  metricsPort: 8080
dataContracts:
  - model: pump
    versions:
      v1:
        name: _pump_v1
        structure:
          temperature:
            _payloadshape: timeseries-number
dataFlow:
  - name: pump-bridge
    desiredState: active
    dataFlowComponentConfig:
      benthos:
        output:
          uns:
            topic: umh.v1.plant._pump_v1.temperature
`)

			// Eventually rather than Expect because GetConfig needs a refresh tick
			// first; this retries until the refusal itself is what comes back.
			Eventually(func() error {
				return configManager.AtomicDeleteDataContract(ctx, "_pump_v1")
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(SatisfyAll(
				MatchError(ContainSubstring("still referenced")),
				MatchError(ContainSubstring("pump-bridge")),
			))

			Expect(writtenData).To(BeEmpty(), "a refused deletion must not write")
		})

		It("reports a name that is not there", func() {
			onDisk(pumpConfig)

			Eventually(func() error {
				return configManager.AtomicDeleteDataContract(ctx, "_absent_v1")
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(MatchError(ContainSubstring("not found")))
		})
	})

	Describe("deleting a whole group", func() {
		It("removes every version and address", func() {
			onDisk(pumpConfig)

			Eventually(func() error {
				return configManager.AtomicDeleteDataContractGroup(ctx, "pump")
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(Succeed())

			Expect(written().Contracts).To(BeEmpty())
		})

		It("refuses while a stream processor declares the model, and names it", func() {
			onDisk(`
agent:
  metricsPort: 8080
dataContracts:
  - model: pump
    versions:
      v1:
        name: _pump_v1
        structure:
          temperature:
            _payloadshape: timeseries-number
streamProcessor:
  - name: pump-processor
    desiredState: active
    streamProcessorServiceConfig:
      config:
        model:
          name: pump
          version: v1
`)

			Eventually(func() error {
				return configManager.AtomicDeleteDataContractGroup(ctx, "pump")
			}, TimeToWaitForConfigRefresh*2, "10ms").Should(SatisfyAll(
				MatchError(ContainSubstring("pump-processor")),
				MatchError(ContainSubstring("streamProcessor")),
				MatchError(ContainSubstring("declares model")),
			))

			Expect(writtenData).To(BeEmpty(), "a refused deletion must not write")
		})
	})

	Describe("version minting", func() {
		// The pre-merge loop, unchanged. Kept as a direct test because minor versions
		// (ENG-5500) will change it and this pins what it does today.
		DescribeTable("nextVersionKey",
			func(existing []string, expected string) {
				contracts := make([]DataContract, 0, len(existing))
				for _, v := range existing {
					contracts = append(contracts, DataContract{Model: "m", Version: v})
				}

				Expect(nextVersionKey(contracts, "m")).To(Equal(expected))
			},
			Entry("no versions yet", nil, "v1"),
			Entry("after v1", []string{"v1"}, "v2"),
			Entry("gaps do not lower it", []string{"v1", "v3"}, "v4"),
			Entry("order does not matter", []string{"v3", "v1"}, "v4"),
			Entry("double digits", []string{"v9", "v10"}, "v11"),
			Entry("unparseable keys are ignored", []string{"draft", "v2"}, "v3"),
			Entry("only unparseable keys", []string{"draft"}, "v1"),
		)

		It("ignores versions belonging to another group", func() {
			contracts := []DataContract{
				{Model: "pump", Version: "v7"},
				{Model: "motor", Version: "v1"},
			}

			Expect(nextVersionKey(contracts, "motor")).To(Equal("v2"))
		})
	})

	Describe("a degraded config", func() {
		// Persisting a change into sections we are no longer deriving from would lose
		// it on the next read, so every mutation refuses instead.
		It("refuses every mutation", func() {
			degraded := FullConfig{ContractsDegraded: true}

			Expect(refuseIfDegraded(degraded)).To(MatchError(errContractsNotLossless))
			Expect(refuseIfDegraded(FullConfig{})).To(Succeed())
		})
	})
})
