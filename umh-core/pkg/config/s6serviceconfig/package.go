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

package s6serviceconfig

import "reflect"

// S6ServiceConfig contains configuration for creating a service.
type S6ServiceConfig struct {
	Env         map[string]string `yaml:"env"`
	ConfigFiles map[string]string `yaml:"configFiles"`
	Command     []string          `yaml:"command"`
	MemoryLimit int64             `yaml:"memoryLimit"` // 0 means no memory limit, see also https://skarnet.org/software/s6/s6-softlimit.html
	LogFilesize int64             `yaml:"logFilesize"` // 0 means default (1MB). Setting smaller values (like 16KB) significantly improves performance for services that regularly read logs. Each log read/parse operation is proportional to file size, so keep this small for monitoring services. Cannot be set lower than 4096 or higher than 268435455, see also https://skarnet.org/software/s6/s6-log.html
}

// Equal checks if two S6ServiceConfigs are equal.
func (c S6ServiceConfig) Equal(other S6ServiceConfig) bool {
	return reflect.DeepEqual(c, other)
}
