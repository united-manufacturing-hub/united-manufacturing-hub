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

package s6_default

import "errors"

var (
	// ErrServiceNotExist indicates the requested service does not exist.
	ErrServiceNotExist = errors.New("service does not exist")

	// ErrServiceConfigMapNotFound indicates the service config map was not found.
	ErrServiceConfigMapNotFound = errors.New("service config map not found")

	// ErrInvalidStatus is returned when the status file has an invalid format.
	ErrInvalidStatus = errors.New("invalid status file format")

	// ErrLogFileNotFound indicates the log file was not found.
	ErrLogFileNotFound = errors.New("log file not found")

	// ErrS6TemporaryError is returned when a s6 command exits with code 111, indicating a temporary error.
	ErrS6TemporaryError = errors.New("s6 temporary error (exit code 111)")

	// ErrS6PermanentError is returned when a s6 command exits with code 100, indicating a permanent error such as misuse.
	ErrS6PermanentError = errors.New("s6 permanent error (exit code 100)")

	// ErrS6ProgramNotFound is returned when a s6 command exits with code 127, indicating it cannot find the program to execute.
	ErrS6ProgramNotFound = errors.New("s6 program not found (exit code 127)")

	// ErrS6ProgramNotExecutable is returned when a s6 command exits with code 126, indicating it failed to execute the program.
	ErrS6ProgramNotExecutable = errors.New("s6 program not executable (exit code 126)")
)
