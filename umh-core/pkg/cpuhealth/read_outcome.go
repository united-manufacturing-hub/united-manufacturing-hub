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

// Read outcomes: the vocabulary for WHY one file read failed. Every reader in
// this package returns its cause — an errno-carrying error from the
// filesystem, or one of the two sentinels here for the failures that have no
// errno — so a caller can report the reason instead of only the fact that a
// read did not work.

package cpuhealth

import (
	"errors"
	"io/fs"
)

// ReadOutcome names the cause of a single file read. It is a string so the
// value can be reported as-is.
type ReadOutcome string

const (
	// ReadOK means the file was read and its content parsed.
	ReadOK ReadOutcome = "ok"
	// ReadENOENT means the file does not exist.
	ReadENOENT ReadOutcome = "enoent"
	// ReadEACCES means the file exists but could not be opened.
	ReadEACCES ReadOutcome = "eacces"
	// ReadEmpty means the file was read and held nothing.
	ReadEmpty ReadOutcome = "empty"
	// ReadUnparsable means content was present but did not parse.
	ReadUnparsable ReadOutcome = "unparsable"
	// ReadError means the read failed for a reason none of the others name. It
	// is never a guess: an error that does not match a known cause classifies
	// here rather than to the nearest-looking one.
	ReadError ReadOutcome = "error"
	// ReadNotAttempted means no read happened, so there is no outcome to
	// report — a cached sticky fact republished without re-reading its file.
	ReadNotAttempted ReadOutcome = "not_attempted"
)

// The two failures that carry no errno. A zero-byte cpuset and a cpu.pressure
// whose avg60 will not parse both read successfully at the syscall layer, so
// neither has a filesystem error to return, and returning nil would report a
// failed read as a good one.
var (
	errEmptyRead      = errors.New("cpuhealth: file empty")
	errUnparsableRead = errors.New("cpuhealth: content did not parse")
)

// classifyRead names the cause of err. It is total: every error classifies,
// and one it does not recognise classifies as ReadError rather than as the
// closest-looking cause.
func classifyRead(err error) ReadOutcome {
	switch {
	case err == nil:
		return ReadOK
	case errors.Is(err, fs.ErrNotExist):
		return ReadENOENT
	case errors.Is(err, fs.ErrPermission):
		return ReadEACCES
	case errors.Is(err, errEmptyRead):
		return ReadEmpty
	case errors.Is(err, errUnparsableRead):
		return ReadUnparsable
	default:
		return ReadError
	}
}
