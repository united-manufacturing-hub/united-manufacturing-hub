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
	"fmt"

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
func (c *Comparator) ConfigsEqual(desired, observed ProtocolConverterServiceConfigSpec) (isEqual bool) {
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

	// If conversion fails for either config, consider them not equal (fail-safe comparison)
	connectionDTemplate, errD := connectionserviceconfig.ConvertTemplateToRuntime(connectionD)
	connectionOTemplate, errO := connectionserviceconfig.ConvertTemplateToRuntime(connectionO)

	// If either conversion fails, they are not equal
	if errD != nil || errO != nil {
		return false
	}

	return comparatorConnection.ConfigsEqual(connectionDTemplate, connectionOTemplate) &&
		comparatorDFC.ConfigsEqual(dfcReadD, dfcReadO) &&
		comparatorDFC.ConfigsEqual(dfcWriteD, dfcWriteO) &&
		comparatorVariable.ConfigsEqual(desired.Variables, observed.Variables)
}

// ConfigDiff returns a human-readable string describing differences between configs
func (c *Comparator) ConfigDiff(desired, observed ProtocolConverterServiceConfigSpec) string {
	connectionD := desired.GetConnectionServiceConfig()
	connectionO := observed.GetConnectionServiceConfig()

	dfcReadD := desired.GetDFCReadServiceConfig()
	dfcReadO := observed.GetDFCReadServiceConfig()

	dfcWriteD := desired.GetDFCWriteServiceConfig()
	dfcWriteO := observed.GetDFCWriteServiceConfig()

	// diff for connection
	comparatorConnection := connectionserviceconfig.NewComparator()
	connectionDTemplate, errD := connectionserviceconfig.ConvertTemplateToRuntime(connectionD)
	connectionOTemplate, errO := connectionserviceconfig.ConvertTemplateToRuntime(connectionO)

	var connectionDiff string
	if errD != nil || errO != nil {
		connectionDiff = fmt.Sprintf("Connection conversion error - desired: %v, observed: %v\n", errD, errO)
	} else {
		connectionDiff = comparatorConnection.ConfigDiff(connectionDTemplate, connectionOTemplate)
	}

	// diff for dfc's
	comparatorDFC := dataflowcomponentserviceconfig.NewComparator()
	dfcReadDiff := comparatorDFC.ConfigDiff(dfcReadD, dfcReadO)
	dfcWriteDiff := comparatorDFC.ConfigDiff(dfcWriteD, dfcWriteO)

	// diff for variables
	comparatorVariable := variables.NewComparator()
	variableDiff := comparatorVariable.ConfigDiff(desired.Variables, observed.Variables)

	return connectionDiff + dfcReadDiff + dfcWriteDiff + variableDiff
}
