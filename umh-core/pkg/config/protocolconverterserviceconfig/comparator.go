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

package protocolconverterserviceconfig

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
)

// Comparator handles the comparison of Connection configurations
type Comparator struct {
}

// NewComparator creates a new configuration comparator for Connection
func NewComparator() *Comparator {
	return &Comparator{}
}

// ConfigsEqual compares two ConnectionServiceConfigs by converting to NmapServiceConfig
// and using the existing comparison utilization
func (c *Comparator) ConfigsEqual(desired, observed ProtocolConverterServiceConfig) (isEqual bool) {
	connectionD := desired.GetConnectionServiceConfig()
	connectionO := observed.GetConnectionServiceConfig()

	dfcReadD := desired.GetDFCReadServiceConfig()
	dfcReadO := observed.GetDFCReadServiceConfig()

	dfcWriteD := desired.GetDFCWriteServiceConfig()
	dfcWriteO := observed.GetDFCWriteServiceConfig()

	// compare connections
	comparatorConnection := connectionserviceconfig.NewComparator()

	// compare dfc's
	comparatorDFC := dataflowcomponentserviceconfig.NewComparator()

	// compare variables
	comparatorVariable := variables.NewComparator()

	return comparatorConnection.ConfigsEqual(connectionD, connectionO) &&
		comparatorDFC.ConfigsEqual(dfcReadD, dfcReadO) &&
		comparatorDFC.ConfigsEqual(dfcWriteD, dfcWriteO) &&
		comparatorVariable.ConfigsEqual(desired.Variables, observed.Variables)
}

// ConfigDiff returns a human-readable string describing differences between configs
func (c *Comparator) ConfigDiff(desired, observed ProtocolConverterServiceConfig) string {
	connectionD := desired.GetConnectionServiceConfig()
	connectionO := observed.GetConnectionServiceConfig()

	dfcReadD := desired.GetDFCReadServiceConfig()
	dfcReadO := observed.GetDFCReadServiceConfig()

	dfcWriteD := desired.GetDFCWriteServiceConfig()
	dfcWriteO := observed.GetDFCWriteServiceConfig()

	// diff for connection
	comparatorConnection := connectionserviceconfig.NewComparator()
	connectionDiff := comparatorConnection.ConfigDiff(connectionD, connectionO)

	// diff for dfc's
	comparatorDFC := dataflowcomponentserviceconfig.NewComparator()
	dfcReadDiff := comparatorDFC.ConfigDiff(dfcReadD, dfcReadO)
	dfcWriteDiff := comparatorDFC.ConfigDiff(dfcWriteD, dfcWriteO)

	// diff for variables
	comparatorVariable := variables.NewComparator()
	variableDiff := comparatorVariable.ConfigDiff(desired.Variables, observed.Variables)

	return connectionDiff + dfcReadDiff + dfcWriteDiff + variableDiff
}
