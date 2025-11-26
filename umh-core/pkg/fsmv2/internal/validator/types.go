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

package validator

import "fmt"

// Violation represents an architectural violation found in code.
type Violation struct {
	File    string
	Line    int
	Type    string
	Message string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d [%s] %s", v.File, v.Line, v.Type, v.Message)
}

// PatternInfo contains metadata about an architectural pattern including WHY it matters.
type PatternInfo struct {
	Name          string
	Why           string
	CorrectCode   string
	ReferenceFile string
}
