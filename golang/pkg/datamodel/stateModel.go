// Copyright 2023 UMH Systems GmbH
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

package datamodel

const (
	// ProducingAtFullSpeedState means that the asset is producing at full speed
	ProducingAtFullSpeedState = 10000

	// ProducingAtLowerThanFullSpeedState means that the asset is producing lower than full speed
	ProducingAtLowerThanFullSpeedState = 20000

	// UnknownState means that the asset is in an unknown state
	UnknownState = 30000

	// UnspecifiedStopState means that the asset is in an unspecified stop
	UnspecifiedStopState = 40000

	// IdleState means that the asset is in an unspecified state, but theoretically ready to run
	IdleState = 40100

	// OperatorInteractionState means that the asset is in an unspecified state, because the operator has stopped the asset manually
	OperatorInteractionState = 40200

	// MicrostopState means that the asset is in an unspecified stop shorter than a certain amount of time
	// (typically between 2 to 10 minutes)
	MicrostopState = 50000

	// InletJamState means that the asset has a jam in the inlet
	InletJamState = 60000

	// OutletJamState means that the asset has a jam in the outlet
	OutletJamState = 70000

	// CongestionBypassState means that the asset has a congestion or deficiency in the bypass flow
	CongestionBypassState = 80000

	// MissingBottleCapsRinneState means that the asset has a congestion or deficiency in the bypass flow, specifically "Kronkorken Rinne"
	MissingBottleCapsRinneState = 80001

	// MissingBottleCapsUebergabeState means that the asset has a congestion or deficiency in the bypass flow, specifically "Kronkorken Uebergabe"
	MissingBottleCapsUebergabeState = 80002

	// MaterialIssueOtherState means that the asset has unknown or other material issue, which is not further specified
	MaterialIssueOtherState = 90000

	// ChangeoverState means that the asset is currently undergoing a changeover
	ChangeoverState = 100000

	// ChangeoverPreparationState means that the asset is currently undergoing a changeover in the preparation step
	// (time between order is started till the machine is first running)
	ChangeoverPreparationState = 100010

	// ChangeoverPostprocessingState means that the asset is currently undergoing a changeover in the postprocessing step
	// (time between machien was last running and the time when the order is ended )
	ChangeoverPostprocessingState = 100020

	// CleaningState means that the asset is currently undergoing a cleaning process
	CleaningState = 110000

	// EmptyingState means that the asset is currently undergoing a emptying process
	EmptyingState = 120000

	// SettingUpState means that the asset is currently setting up (e.g. warming up)
	SettingUpState = 130000

	// OperatorNotAtMachineState means that the asset is not running because there is no operator available
	OperatorNotAtMachineState = 140000

	// OperatorBreakState means that the asset is not running because the operators are in a break
	OperatorBreakState = 150000

	// NoShiftState means that there is no planned shift at the asset
	NoShiftState = 160000

	// NoOrderState means that there is no order at the asset
	NoOrderState = 170000

	// EquipmentFailureState means that there is an equipment failure (e.g. broken engine)
	EquipmentFailureState = 180000

	// EquipmentFailureState means that there is an equipment failure at the welder
	EquipmentFailureStateWelder = 180001

	// EquipmentFailureState means that there is an equipment failure at the welder
	EquipmentFailureStateExpender = 180002

	// EquipmentFailureState means that there is an equipment failure Palletizer
	EquipmentFailureStatePalletizer = 180003

	// EquipmentFailureState means that there is an equipment failure Underbody-machine
	EquipmentFailureStateUnderbody = 180004

	// EquipmentFailureState means that there is an equipment failure (e.g. broken engine)
	EquipmentFailureStateTopcover = 180005

	// ExternalFailureState means that there is an external failure (e.g. missing compressed air)
	ExternalFailureState = 190000

	// ExternalInterferenceState means that the asset is not running because of an external interference
	ExternalInterferenceState = 200000

	// CraneNotAvailableState means that the asset is not running because the crane is currently not available
	CraneNotAvailableState = 200010

	// PreventiveMaintenanceStop means that the asset is currently undergoing a preventive maintenance action
	PreventiveMaintenanceStop = 210000

	// TechnicalOtherStop means that the asset is currently having a not further specified technical issue
	TechnicalOtherStop = 220000

	// MaxState is the highest possible state
	MaxState = 230000
)

// IsSpecifiedStop checks whether the asset is in an specified stop (is not running, is not a microstop or unknown stop, and is defined)
func IsSpecifiedStop(state int) (returnValue bool) {
	if !IsProducing(state) && !IsUnspecifiedStop(state) && !IsMicrostop(state) && !IsUnknown(state) { // new data model
		returnValue = true
	}
	return
}

// IsProducing checks whether the asset is producing
func IsProducing(state int) (returnValue bool) {
	if IsProducingFullSpeed(state) || IsProducingLowerThanFullSpeed(state) { // new data model
		returnValue = true
	}
	return
}

// IsProducingFullSpeed checks whether the asset is producing on the full speed
func IsProducingFullSpeed(state int) (returnValue bool) {
	if state >= ProducingAtFullSpeedState && state <= ProducingAtFullSpeedState+9999 { // new data model
		returnValue = true
	} else if state == 0 { // old data model
		returnValue = true
	}
	return
}

// IsProducingLowerThanFullSpeed checks whether the asset is producing lower than the full speed
func IsProducingLowerThanFullSpeed(state int) (returnValue bool) {
	if state >= ProducingAtLowerThanFullSpeedState && state <= ProducingAtLowerThanFullSpeedState+9999 { // new data model
		returnValue = true
	} else if state == 19 { // old data model
		returnValue = true
	}
	return
}

// IsUnknown checks whether the asset is in an unknown state (e.g. data missing)
func IsUnknown(state int) (returnValue bool) {
	if state >= UnknownState && state <= UnknownState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsUnspecifiedStop checks whether the asset is in an unspecified stop
func IsUnspecifiedStop(state int) (returnValue bool) {
	if state >= UnspecifiedStopState && state <= UnspecifiedStopState+9999 { // new data model
		returnValue = true
	} else if state == 1 { // old data model
		returnValue = true
	}
	return
}

// IsMicrostop checks whether the asset is in a microstop
func IsMicrostop(state int) (returnValue bool) {
	if state >= MicrostopState && state <= MicrostopState+9999 { // new data model
		returnValue = true
	} else if state == 7 { // old data model
		returnValue = true
	}
	return
}

// IsInletJam checks whether
func IsInletJam(state int) (returnValue bool) {
	if state >= InletJamState && state <= InletJamState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsOutletJam checks whether
func IsOutletJam(state int) (returnValue bool) {
	if state >= OutletJamState && state <= OutletJamState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsCongestionBypass checks whether
func IsCongestionBypass(state int) (returnValue bool) {
	if state >= CongestionBypassState && state <= CongestionBypassState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsMaterialIssueOther checks whether
func IsMaterialIssueOther(state int) (returnValue bool) {
	if state >= MaterialIssueOtherState && state <= MaterialIssueOtherState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsChangeover checks whether
func IsChangeover(state int) (returnValue bool) {
	if state >= ChangeoverState && state <= ChangeoverState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsCleaning checks whether
func IsCleaning(state int) (returnValue bool) {
	if state >= CleaningState && state <= CleaningState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsEmptying checks whether
func IsEmptying(state int) (returnValue bool) {
	if state >= EmptyingState && state <= EmptyingState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsSettingUp checks whether
func IsSettingUp(state int) (returnValue bool) {
	if state >= SettingUpState && state <= SettingUpState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsOperatorNotAtMachine checks whether
func IsOperatorNotAtMachine(state int) (returnValue bool) {
	if state >= OperatorNotAtMachineState && state <= OperatorNotAtMachineState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsOperatorBreak checks whether the asset is not running because the operators are in a break
func IsOperatorBreak(state int) (returnValue bool) {
	if state >= OperatorBreakState && state <= OperatorBreakState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsNoShift checks whether the asset has no shift planned
func IsNoShift(state int) (returnValue bool) {
	if state >= NoShiftState && state <= NoShiftState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsNoOrder checks whether
func IsNoOrder(state int) (returnValue bool) {
	if state >= NoOrderState && state <= NoOrderState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsEquipmentFailure checks whether
func IsEquipmentFailure(state int) (returnValue bool) {
	if state >= EquipmentFailureState && state <= EquipmentFailureState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsExternalFailure checks whether
func IsExternalFailure(state int) (returnValue bool) {
	if state >= ExternalFailureState && state <= ExternalFailureState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsExternalInterference checks whether
func IsExternalInterference(state int) (returnValue bool) {
	if state >= ExternalInterferenceState && state <= ExternalInterferenceState+9999 { // new data model
		returnValue = true
	}
	return
}

// IsPreventiveMaintenance checks whether
func IsPreventiveMaintenance(state int) (returnValue bool) {
	if state >= PreventiveMaintenanceStop && state <= PreventiveMaintenanceStop+9999 { // new data model
		returnValue = true
	}
	return
}

// IsTechnicalOtherStop checks whether
func IsTechnicalOtherStop(state int) (returnValue bool) {
	if state >= TechnicalOtherStop && state <= TechnicalOtherStop+9999 { // new data model
		returnValue = true
	}
	return
}
