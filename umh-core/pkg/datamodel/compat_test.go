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

	It("errors on a _refModel field when allModels is nil rather than dropping it silently", func() {
		_, err := datamodel.FlattenResolvedForTest(context.Background(),
			config.DataModelVersion{Structure: map[string]config.Field{
				"motor": {ModelRef: &config.ModelRef{Name: "motor", Version: "v1_0"}},
			}}, nil, nil)
		Expect(err).To(HaveOccurred(),
			"a _refModel field must never vanish from the flattened output without a tag or an error, or CheckAdditive sees a false removal")
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

var _ = Describe("CheckAdditive", func() {
	ctx := context.Background()
	number := config.Field{PayloadShape: "timeseries-number"}
	text := config.Field{PayloadShape: "timeseries-string"}

	version := func(fields map[string]config.Field) config.DataModelVersion {
		return config.DataModelVersion{Structure: fields}
	}

	It("reports no violation for an identical version (P3)", func() {
		v := version(map[string]config.Field{"temperature": number})
		changes, err := datamodel.CheckAdditive(ctx, v, v, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(BeEmpty())
	})

	It("allows a pure addition (P4)", func() {
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"temperature": number}),
			version(map[string]config.Field{"temperature": number, "pressure": number}),
			nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(BeEmpty())
	})

	It("reports a removal", func() {
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"temperature": number, "rpm": number}),
			version(map[string]config.Field{"temperature": number}),
			nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		Expect(changes[0].Path).To(Equal("rpm"))
		Expect(changes[0].Kind).To(Equal(datamodel.Removed))
	})

	It("reports a retype, with an empty payloadShapes map (P7)", func() {
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"temperature": number}),
			version(map[string]config.Field{"temperature": text}),
			nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		Expect(changes[0].Path).To(Equal("temperature"))
		Expect(changes[0].Kind).To(Equal(datamodel.Retyped))
		Expect(changes[0].OldShape).To(Equal("timeseries-number"))
		Expect(changes[0].NewShape).To(Equal("timeseries-string"))
	})

	It("reports a rename as a removal plus an allowed addition", func() {
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"temp": number}),
			version(map[string]config.Field{"temperature": number}),
			nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		Expect(changes[0].Path).To(Equal("temp"))
		Expect(changes[0].Kind).To(Equal(datamodel.Removed))
	})

	It("reports a leaf turning into a folder", func() {
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"motor": number}),
			version(map[string]config.Field{"motor": {Subfields: map[string]config.Field{"rpm": number}}}),
			nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		Expect(changes[0].Path).To(Equal("motor"))
		Expect(changes[0].Kind).To(Equal(datamodel.Removed))
	})

	It("reports a folder turning into a leaf", func() {
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"motor": {Subfields: map[string]config.Field{"rpm": number}}}),
			version(map[string]config.Field{"motor": number}),
			nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		Expect(changes[0].Path).To(Equal("motor.rpm"))
		Expect(changes[0].Kind).To(Equal(datamodel.Removed))
	})

	It("reports a relational field whose inner type changed", func() {
		rel := func(t string) config.Field {
			return config.Field{Relational: &config.PayloadShape{
				Fields: map[string]config.PayloadField{"a": {Type: t}},
			}}
		}
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"r": rel("number")}),
			version(map[string]config.Field{"r": rel("string")}),
			nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		Expect(changes[0].Kind).To(Equal(datamodel.Retyped))
	})

	It("reports every violation, not just the first", func() {
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"a": number, "b": number, "c": number}),
			version(map[string]config.Field{"a": text, "c": number}),
			nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(2))
	})

	It("returns violations in a stable order", func() {
		prev := version(map[string]config.Field{"z": number, "a": number, "m": number})
		next := version(map[string]config.Field{})
		first, err := datamodel.CheckAdditive(ctx, prev, next, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		for range 5 {
			again, err := datamodel.CheckAdditive(ctx, prev, next, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(again).To(Equal(first))
		}
	})

	It("guarantees every prev path survives when it reports nothing (P5)", func() {
		prev := version(map[string]config.Field{"a": number, "b": text})
		next := version(map[string]config.Field{"a": number, "b": text, "c": number})

		changes, err := datamodel.CheckAdditive(ctx, prev, next, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(BeEmpty())

		prevFlat, err := datamodel.FlattenResolvedForTest(ctx, prev, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		nextFlat, err := datamodel.FlattenResolvedForTest(ctx, next, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		for path, tag := range prevFlat {
			Expect(nextFlat).To(HaveKey(path))
			Expect(datamodel.ShapesEqualForTest(tag.Shape, nextFlat[path].Shape)).To(BeTrue())
		}
	})

	It("allows a refModel bumped to a newer minor that only adds", func() {
		models := map[string]config.DataModelsConfig{
			"motor": {Name: "motor", Versions: map[string]config.DataModelVersion{
				"v1_0": version(map[string]config.Field{"rpm": number}),
				"v1_1": version(map[string]config.Field{"rpm": number, "torque": number}),
			}},
		}
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"m": {ModelRef: &config.ModelRef{Name: "motor", Version: "v1_0"}}}),
			version(map[string]config.Field{"m": {ModelRef: &config.ModelRef{Name: "motor", Version: "v1_1"}}}),
			models, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(BeEmpty())
	})

	It("refuses a refModel swapped to a major that dropped a tag", func() {
		models := map[string]config.DataModelsConfig{
			"motor": {Name: "motor", Versions: map[string]config.DataModelVersion{
				"v1_0": version(map[string]config.Field{"rpm": number, "torque": number}),
				"v2_0": version(map[string]config.Field{"rpm": number}),
			}},
		}
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"m": {ModelRef: &config.ModelRef{Name: "motor", Version: "v1_0"}}}),
			version(map[string]config.Field{"m": {ModelRef: &config.ModelRef{Name: "motor", Version: "v2_0"}}}),
			models, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(HaveLen(1))
		Expect(changes[0].Path).To(Equal("m.torque"))
	})

	It("allows inlining a reference when every tag is preserved", func() {
		models := map[string]config.DataModelsConfig{
			"motor": {Name: "motor", Versions: map[string]config.DataModelVersion{
				"v1_0": version(map[string]config.Field{"rpm": number}),
			}},
		}
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"m": {ModelRef: &config.ModelRef{Name: "motor", Version: "v1_0"}}}),
			version(map[string]config.Field{"m": {Subfields: map[string]config.Field{"rpm": number}}}),
			models, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(changes).To(BeEmpty())
	})

	It("errors when the predecessor cannot be flattened (P9)", func() {
		changes, err := datamodel.CheckAdditive(ctx,
			version(map[string]config.Field{"m": {ModelRef: &config.ModelRef{Name: "gone", Version: "v1_0"}}}),
			version(map[string]config.Field{"temperature": number}),
			map[string]config.DataModelsConfig{}, nil)
		Expect(err).To(HaveOccurred())
		Expect(changes).To(BeEmpty(), "an error must never come back as an empty, passing violation list")
	})
})

var _ = Describe("FormatBreakingChanges", func() {
	It("names the model, the version and every change", func() {
		msg := datamodel.FormatBreakingChanges("pump", "v1_1", []datamodel.BreakingChange{
			{Path: "motor.rpm", Kind: datamodel.Removed, OldShape: "timeseries-number"},
			{Path: "pressure", Kind: datamodel.Retyped, OldShape: "timeseries-number", NewShape: "timeseries-string"},
		})
		Expect(msg).To(ContainSubstring(`cannot add version v1_1 to data model "pump"`))
		Expect(msg).To(ContainSubstring("2 breaking changes"))
		Expect(msg).To(ContainSubstring("motor.rpm"))
		Expect(msg).To(ContainSubstring("removed"))
		Expect(msg).To(ContainSubstring("timeseries-number -> timeseries-string"))
		Expect(msg).To(ContainSubstring("not supported yet"))
	})

	It("says the relational field's definition changed instead of printing the same shape name twice", func() {
		msg := datamodel.FormatBreakingChanges("pump", "v1_1", []datamodel.BreakingChange{
			{Path: "r", Kind: datamodel.Retyped, OldShape: "__relational_r__", NewShape: "__relational_r__"},
		})
		Expect(msg).To(ContainSubstring("r  relational field definition changed"))
		Expect(msg).NotTo(ContainSubstring("__relational_r__ -> __relational_r__"))
	})
})
