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

package secrets_test

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps/secrets"
)

// planted is the string the printing and encoding assertions expect to stay
// out of output; the Reveal assertion checks it comes back unchanged.
const planted = "p1aNt3d-S3cr3t-Xq9"

var _ = Describe("Value", func() {
	It("checks that json encoding fails for a value and for a struct field", func() {
		v := secrets.NewValue(planted)

		_, err := json.Marshal(v)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, secrets.ErrNotSerializable)).To(BeTrue())

		_, err = json.Marshal(struct{ S secrets.Value }{v})
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, secrets.ErrNotSerializable)).To(BeTrue())
	})

	It("checks that yaml, text and gob encoding each fail, and gob writes no bytes of the value", func() {
		v := secrets.NewValue(planted)

		_, err := yaml.Marshal(v)
		Expect(err).To(HaveOccurred())

		_, err = v.MarshalText()
		Expect(err).To(HaveOccurred())

		var buf bytes.Buffer

		err = gob.NewEncoder(&buf).Encode(v)
		Expect(err).To(HaveOccurred())
		Expect(buf.String()).NotTo(ContainSubstring(planted))
	})

	It("checks that every fmt verb prints the placeholder instead of the value", func() {
		v := secrets.NewValue(planted)

		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
			for _, operand := range []any{any(v), any(&v)} {
				out := fmt.Sprintf(verb, operand)
				Expect(out).To(ContainSubstring("[secret]"), "for verb "+verb)
				Expect(out).NotTo(ContainSubstring(planted), "for verb "+verb)
			}
		}

		out := fmt.Errorf("x: %v", v).Error()
		Expect(out).To(ContainSubstring("[secret]"))
		Expect(out).NotTo(ContainSubstring(planted))
	})

	It("checks that zap logging writes neither the value nor its encoded forms", func() {
		obsCore, logs := observer.New(zapcore.DebugLevel)

		var buf bytes.Buffer

		jsonCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(&buf),
			zapcore.DebugLevel,
		)
		sugar := zap.New(zapcore.NewTee(obsCore, jsonCore)).Sugar()
		fsm := deps.NewUnsampledFSMLogger(sugar)

		v := secrets.NewValue(planted)
		sugar.Desugar().Debug("m", zap.Any("k", v))
		sugar.Desugar().Debug("m", zap.Reflect("k", v))
		fsm.Debug("m", deps.Any("k", v))

		Expect(logs.Len()).To(Equal(3))

		out := buf.String()
		Expect(out).NotTo(ContainSubstring(planted))
		Expect(out).NotTo(ContainSubstring(base64.StdEncoding.EncodeToString([]byte(planted))))
		Expect(out).NotTo(ContainSubstring(base64.URLEncoding.EncodeToString([]byte(planted))))
		Expect(out).NotTo(ContainSubstring(hex.EncodeToString([]byte(planted))))
		Expect(out).NotTo(ContainSubstring(strings.ToUpper(hex.EncodeToString([]byte(planted)))))
	})

	It("checks that a struct with a *Value field does not print the held string", func() {
		type withPointer struct {
			v *secrets.Value
		}

		v := secrets.NewValue(planted)
		out := fmt.Sprintf("%+v", withPointer{v: &v})
		Expect(out).NotTo(ContainSubstring(planted))
	})

	It("checks that a struct with a direct value field prints no bytes of the value", func() {
		type withValue struct {
			v secrets.Value
		}

		type withExported struct {
			V secrets.Value
		}

		o := withValue{v: secrets.NewValue(planted)}
		e := withExported{V: secrets.NewValue(planted)}

		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
			out := fmt.Sprintf(verb, o)
			Expect(out).NotTo(ContainSubstring(planted), "unexported field, for verb "+verb)
			out = fmt.Sprintf(verb, e)
			Expect(out).NotTo(ContainSubstring(planted), "exported field, for verb "+verb)
		}
	})

	It("checks that Gomega's object formatter writes no bytes of the value", func() {
		v := secrets.NewValue(planted)

		out := format.Object(v, 0)
		Expect(out).NotTo(ContainSubstring(planted))

		out = format.Object(struct{ v secrets.Value }{v}, 0)
		Expect(out).NotTo(ContainSubstring(planted))
	})

	It("checks that Reveal returns the held string", func() {
		Expect(secrets.NewValue(planted).Reveal()).To(Equal(planted))
	})
})
