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

package secrets

import (
	"errors"
	"fmt"
	"io"
)

// ErrNotSerializable is returned by encoders that would write the held string.
var ErrNotSerializable = errors.New("secrets: value cannot be encoded")

// placeholder is what a Value prints instead of the string it holds.
const placeholder = "[secret]"

// box holds the string inside a closure, so fmt has no string field to print:
// it cannot call Format or String through an unexported field of another
// struct, so what it prints is the closure's function address.
type box struct {
	reveal func() string
}

// Value holds a string that must not appear in logs or serialized state.
// The string sits behind a closure in an unexported struct, and Format prints
// the placeholder for every verb, so no fmt verb or log field can write it.
// Values are immutable, and Get and the store share the same box, so no
// method may change one in place.
type Value struct {
	p *box
}

// NewValue builds a Value holding s.
func NewValue(s string) Value {
	return Value{p: &box{reveal: func() string { return s }}}
}

// Reveal returns the held string.
func (v Value) Reveal() string {
	if v.p == nil {
		return ""
	}
	return v.p.reveal()
}

// Format prints the placeholder for every verb. fmt calls it for every verb
// except %T and %p; %T prints the type name and %p an address, and neither
// writes the string.
func (v Value) Format(f fmt.State, verb rune) {
	_, _ = io.WriteString(f, placeholder)
}

// MarshalText refuses to write the held string.
func (v Value) MarshalText() ([]byte, error) {
	return nil, ErrNotSerializable
}
