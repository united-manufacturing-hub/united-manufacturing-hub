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

package handoff_test

import (
	"reflect"
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/handoff"
)

func TestMessageFields(t *testing.T) {
	typ := reflect.TypeFor[handoff.Message]()

	if typ.NumField() != 3 {
		t.Fatalf("Message has %d fields, want 3", typ.NumField())
	}

	for i, want := range []string{"Content", "SenderEmail", "TraceID"} {
		field := typ.Field(i)
		if field.Name != want {
			t.Errorf("field %d is %s, want %s", i, field.Name, want)
		}
		if field.PkgPath != "" {
			t.Errorf("field %s is not exported", field.Name)
		}
		if field.Tag != "" {
			t.Errorf("field %s has struct tag %q, want none", field.Name, field.Tag)
		}
	}

	if got := typ.Field(0).Type; got != reflect.TypeFor[[]byte]() {
		t.Errorf("Content type is %s, want []byte", got)
	}
	if got := typ.Field(1).Type.Kind(); got != reflect.String {
		t.Errorf("SenderEmail kind is %s, want string", got)
	}
	if got := typ.Field(2).Type.Kind(); got != reflect.String {
		t.Errorf("TraceID kind is %s, want string", got)
	}
}
