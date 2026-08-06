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

package datamodel_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/datamodel"
)

var _ = Describe("flattenResolved", func() {
	numberTag := config.Field{PayloadShape: "timeseries-number"}
	stringTag := config.Field{PayloadShape: "timeseries-string"}

	It("resolves default shapes even when payloadShapes is empty (P7)", func() {
		got, err := datamodel.FlattenResolvedForTest(context.Background(),
			config.DataModelVersion{Structure: map[string]config.Field{
				"temperature": numberTag,
				"label":       stringTag,
			}},
			nil,
			nil, // no payloadShapes at all, as in every real config
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(HaveKey("temperature"))
		Expect(got).To(HaveKey("label"))

		Expect(datamodel.ShapesEqualForTest(got["temperature"].Shape, got["label"].Shape)).To(BeFalse(),
			"a number shape and a string shape must not compare equal, or no retype is ever detected")
	})

	It("errors on an unresolvable shape name rather than returning the zero shape", func() {
		_, err := datamodel.FlattenResolvedForTest(context.Background(),
			config.DataModelVersion{Structure: map[string]config.Field{
				"x": {PayloadShape: "no-such-shape"},
			}}, nil, nil)
		Expect(err).To(HaveOccurred())
	})

	It("errors when a reference dangles rather than returning an empty map (P9)", func() {
		_, err := datamodel.FlattenResolvedForTest(context.Background(),
			config.DataModelVersion{Structure: map[string]config.Field{
				"motor": {ModelRef: &config.ModelRef{Name: "missing", Version: "v1_0"}},
			}}, map[string]config.DataModelsConfig{}, nil)
		Expect(err).To(HaveOccurred())
	})

	It("does not mutate the payloadShapes argument (P8)", func() {
		shapes := map[string]config.PayloadShape{}
		_, err := datamodel.FlattenResolvedForTest(context.Background(),
			config.DataModelVersion{Structure: map[string]config.Field{
				"rel": {Relational: &config.PayloadShape{
					Fields: map[string]config.PayloadField{"a": {Type: "number"}},
				}},
			}}, nil, shapes)
		Expect(err).NotTo(HaveOccurred())
		Expect(shapes).To(BeEmpty(), "the synthetic relational shape must not leak into the caller's map")
	})

	It("distinguishes two relational shapes at the same path", func() {
		build := func(fieldType string) config.DataModelVersion {
			return config.DataModelVersion{Structure: map[string]config.Field{
				"rel": {Relational: &config.PayloadShape{
					Fields: map[string]config.PayloadField{"a": {Type: fieldType}},
				}},
			}}
		}

		first, err := datamodel.FlattenResolvedForTest(context.Background(), build("number"), nil, nil)
		Expect(err).NotTo(HaveOccurred())
		second, err := datamodel.FlattenResolvedForTest(context.Background(), build("string"), nil, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(datamodel.ShapesEqualForTest(first["rel"].Shape, second["rel"].Shape)).To(BeFalse(),
			"the synthetic shape name is path-derived and identical, so only the definition distinguishes them")
	})

	It("flattens a reference into the referenced model's tags", func() {
		got, err := datamodel.FlattenResolvedForTest(context.Background(),
			config.DataModelVersion{Structure: map[string]config.Field{
				"motor": {ModelRef: &config.ModelRef{Name: "motor", Version: "v1_0"}},
			}},
			map[string]config.DataModelsConfig{
				"motor": {Name: "motor", Versions: map[string]config.DataModelVersion{
					"v1_0": {Structure: map[string]config.Field{"rpm": numberTag}},
				}},
			}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(HaveKey("motor.rpm"))
	})
})
