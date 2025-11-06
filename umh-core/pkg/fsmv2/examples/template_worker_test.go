package examples_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/location"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
)

var _ = Describe("TemplateWorker", func() {
	var (
		worker         *examples.TemplateWorker
		parentLocation []location.LocationLevel
		childLocation  []location.LocationLevel
	)

	BeforeEach(func() {
		parentLocation = []location.LocationLevel{
			{Type: "enterprise", Value: "ACME"},
			{Type: "site", Value: "Factory-1"},
		}
		childLocation = []location.LocationLevel{
			{Type: "line", Value: "Line-A"},
		}
		worker = examples.NewTemplateWorker(parentLocation, childLocation)
	})

	Describe("DeriveDesiredState", func() {
		Context("with user variables", func() {
			It("should render template with flattened user variables", func() {
				userSpec := types.UserSpec{
					Variables: types.VariableBundle{
						User: map[string]any{
							"IP":   "192.168.1.100",
							"PORT": 502,
							"name": "test_bridge",
						},
					},
				}

				desired, err := worker.DeriveDesiredState(userSpec)
				Expect(err).NotTo(HaveOccurred())
				Expect(desired.State).To(Equal("running"))
				Expect(desired.ChildrenSpecs).To(HaveLen(1))

				childSpec := desired.ChildrenSpecs[0]
				Expect(childSpec.Name).To(Equal("mqtt_source"))
				Expect(childSpec.WorkerType).To(Equal("benthos_dataflow"))

				config := childSpec.UserSpec.Config
				Expect(config).To(ContainSubstring("192.168.1.100"))
				Expect(config).To(ContainSubstring("502"))
				Expect(config).NotTo(ContainSubstring("{{ .IP }}"))
				Expect(config).NotTo(ContainSubstring("{{ .PORT }}"))
			})

			It("should use flattened variables not nested user variables", func() {
				userSpec := types.UserSpec{
					Variables: types.VariableBundle{
						User: map[string]any{
							"IP":   "broker",
							"PORT": 1883,
							"name": "mqtt_test",
						},
					},
				}

				desired, err := worker.DeriveDesiredState(userSpec)
				Expect(err).NotTo(HaveOccurred())

				config := desired.ChildrenSpecs[0].UserSpec.Config
				Expect(config).To(ContainSubstring("broker:1883"))
				Expect(config).NotTo(ContainSubstring("{{ .user.IP }}"))
			})

			It("should compute location_path and add to template data", func() {
				userSpec := types.UserSpec{
					Variables: types.VariableBundle{
						User: map[string]any{
							"IP":   "192.168.1.100",
							"PORT": 502,
							"name": "test_bridge",
						},
					},
				}

				desired, err := worker.DeriveDesiredState(userSpec)
				Expect(err).NotTo(HaveOccurred())

				config := desired.ChildrenSpecs[0].UserSpec.Config
				Expect(config).To(ContainSubstring("umh.v1.ACME.Factory-1.Line-A.test_bridge"))
				Expect(config).NotTo(ContainSubstring("{{ .location_path }}"))
			})

			It("should pass rendered config without template syntax", func() {
				userSpec := types.UserSpec{
					Variables: types.VariableBundle{
						User: map[string]any{
							"IP":   "10.0.0.5",
							"PORT": 8080,
							"name": "api_bridge",
						},
					},
				}

				desired, err := worker.DeriveDesiredState(userSpec)
				Expect(err).NotTo(HaveOccurred())

				config := desired.ChildrenSpecs[0].UserSpec.Config
				Expect(config).NotTo(ContainSubstring("{{"))
				Expect(config).NotTo(ContainSubstring("}}"))
				Expect(config).To(ContainSubstring("10.0.0.5:8080"))
			})
		})

		Context("with child variables", func() {
			It("should create child with UserSpec containing variables", func() {
				userSpec := types.UserSpec{
					Variables: types.VariableBundle{
						User: map[string]any{
							"IP":   "192.168.1.100",
							"PORT": 502,
							"name": "modbus_bridge",
						},
					},
				}

				desired, err := worker.DeriveDesiredState(userSpec)
				Expect(err).NotTo(HaveOccurred())

				childSpec := desired.ChildrenSpecs[0]
				Expect(childSpec.UserSpec.Variables.User).To(HaveKeyWithValue("name", "mqtt_source"))
			})
		})

		Context("with template errors", func() {
			It("should return error when variable is missing in strict mode", func() {
				userSpec := types.UserSpec{
					Variables: types.VariableBundle{
						User: map[string]any{
							"IP": "192.168.1.100",
						},
					},
				}

				_, err := worker.DeriveDesiredState(userSpec)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("PORT"))
			})

			It("should return error when template syntax is invalid", func() {
				worker.Template = "{{ .IP"

				userSpec := types.UserSpec{
					Variables: types.VariableBundle{
						User: map[string]any{
							"IP": "192.168.1.100",
						},
					},
				}

				_, err := worker.DeriveDesiredState(userSpec)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with multiple children", func() {
			It("should create multiple children with different templates", func() {
				worker.MultiChild = true

				userSpec := types.UserSpec{
					Variables: types.VariableBundle{
						User: map[string]any{
							"IP":   "192.168.1.100",
							"PORT": 502,
							"name": "test_converter",
						},
					},
				}

				desired, err := worker.DeriveDesiredState(userSpec)
				Expect(err).NotTo(HaveOccurred())
				Expect(desired.ChildrenSpecs).To(HaveLen(2))

				Expect(desired.ChildrenSpecs[0].Name).To(Equal("mqtt_source"))
				Expect(desired.ChildrenSpecs[1].Name).To(Equal("kafka_sink"))

				mqttConfig := desired.ChildrenSpecs[0].UserSpec.Config
				Expect(mqttConfig).To(ContainSubstring("input"))
				Expect(mqttConfig).To(ContainSubstring("192.168.1.100:502"))

				kafkaConfig := desired.ChildrenSpecs[1].UserSpec.Config
				Expect(kafkaConfig).To(ContainSubstring("output"))
				Expect(kafkaConfig).To(ContainSubstring("umh.v1.ACME.Factory-1.Line-A.test_converter"))
			})
		})
	})

	Describe("location path computation", func() {
		It("should merge parent and child locations correctly", func() {
			userSpec := types.UserSpec{
				Variables: types.VariableBundle{
					User: map[string]any{
						"IP":   "192.168.1.100",
						"PORT": 502,
						"name": "bridge",
					},
				},
			}

			desired, err := worker.DeriveDesiredState(userSpec)
			Expect(err).NotTo(HaveOccurred())

			config := desired.ChildrenSpecs[0].UserSpec.Config
			Expect(config).To(ContainSubstring("ACME.Factory-1.Line-A"))
		})

		It("should handle empty parent location", func() {
			worker = examples.NewTemplateWorker([]location.LocationLevel{}, childLocation)

			userSpec := types.UserSpec{
				Variables: types.VariableBundle{
					User: map[string]any{
						"IP":   "192.168.1.100",
						"PORT": 502,
						"name": "bridge",
					},
				},
			}

			desired, err := worker.DeriveDesiredState(userSpec)
			Expect(err).NotTo(HaveOccurred())

			config := desired.ChildrenSpecs[0].UserSpec.Config
			Expect(config).To(ContainSubstring("Line-A"))
			Expect(config).NotTo(ContainSubstring(".."))
		})

		It("should handle empty child location", func() {
			worker = examples.NewTemplateWorker(parentLocation, []location.LocationLevel{})

			userSpec := types.UserSpec{
				Variables: types.VariableBundle{
					User: map[string]any{
						"IP":   "192.168.1.100",
						"PORT": 502,
						"name": "bridge",
					},
				},
			}

			desired, err := worker.DeriveDesiredState(userSpec)
			Expect(err).NotTo(HaveOccurred())

			config := desired.ChildrenSpecs[0].UserSpec.Config
			Expect(config).To(ContainSubstring("ACME.Factory-1"))
			Expect(config).NotTo(ContainSubstring(".."))
		})
	})
})
