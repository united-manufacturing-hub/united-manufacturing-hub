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

var ModelState = newModelStateRegistry()

func newModelStateRegistry() *modelStateRegistry {
	return &modelStateRegistry{
		ProducingAtFullSpeedState:          "ProducingAtFullSpeedState",
		ProducingAtLowerThanFullSpeedState: "ProducingAtLowerThanFullSpeedState",
		UnknownState:                       "UnknownState",
		UnspecifiedStopState:               "UnspecifiedStopState",
		IdleState:                          "IdleState",
		OperatorInteractionState:           "OperatorInteractionState",
		MicrostopState:                     "MicrostopState",
		InletJamState:                      "InletJamState",
		OutletJamState:                     "OutletJamState",
		CongestionBypassState:              "CongestionBypassState",
		MissingBottleCapsRinneState:        "MissingBottleCapsRinneState",
		MissingBottleCapsUebergabeState:    "MissingBottleCapsUebergabeState",
		MaterialIssueOtherState:            "MaterialIssueOtherState",
		ChangeoverState:                    "ChangeoverState",
		ChangeoverPreparationState:         "ChangeoverPreparationState",
		ChangeoverPostprocessingState:      "ChangeoverPostprocessingState",
		CleaningState:                      "CleaningState",
		EmptyingState:                      "EmptyingState",
		SettingUpState:                     "SettingUpState",
		OperatorNotAtMachineState:          "OperatorNotAtMachineState",
		OperatorBreakState:                 "OperatorBreakState",
		NoShiftState:                       "NoShiftState",
		NoOrderState:                       "NoOrderState",
		EquipmentFailureState:              "EquipmentFailureState",
		EquipmentFailureStateWelder:        "EquipmentFailureStateWelder",
		EquipmentFailureStateExpender:      "EquipmentFailureStateExpender",
		EquipmentFailureStatePalletizer:    "EquipmentFailureStatePalletizer",
		EquipmentFailureStateUnderbody:     "EquipmentFailureStateUnderbody",
		EquipmentFailureStateTopcover:      "EquipmentFailureStateTopcover",
		ExternalFailureState:               "ExternalFailureState",
		ExternalInterferenceState:          "ExternalInterferenceState",
		CraneNotAvailableState:             "CraneNotAvailableState",
		PreventiveMaintenanceStop:          "PreventiveMaintenanceStop",
		TechnicalOtherStop:                 "TechnicalOtherStop",
		MaxState:                           "MaxState",
	}
}

type modelStateRegistry struct {
	ProducingAtFullSpeedState          string
	ProducingAtLowerThanFullSpeedState string
	UnknownState                       string
	UnspecifiedStopState               string
	IdleState                          string
	OperatorInteractionState           string
	MicrostopState                     string
	InletJamState                      string
	OutletJamState                     string
	CongestionBypassState              string
	MissingBottleCapsRinneState        string
	MissingBottleCapsUebergabeState    string
	MaterialIssueOtherState            string
	ChangeoverState                    string
	ChangeoverPreparationState         string
	ChangeoverPostprocessingState      string
	CleaningState                      string
	EmptyingState                      string
	SettingUpState                     string
	OperatorNotAtMachineState          string
	OperatorBreakState                 string
	NoShiftState                       string
	NoOrderState                       string
	EquipmentFailureState              string
	EquipmentFailureStateWelder        string
	EquipmentFailureStateExpender      string
	EquipmentFailureStatePalletizer    string
	EquipmentFailureStateUnderbody     string
	EquipmentFailureStateTopcover      string
	ExternalFailureState               string
	ExternalInterferenceState          string
	CraneNotAvailableState             string
	PreventiveMaintenanceStop          string
	TechnicalOtherStop                 string
	MaxState                           string
}

// GetStateFromString returns the state number from its string representation
func GetStateFromString(stateString string) uint64 {

	stateMap := map[string]uint64{

		ModelState.ProducingAtFullSpeedState:          ProducingAtFullSpeedState,
		ModelState.ProducingAtLowerThanFullSpeedState: ProducingAtLowerThanFullSpeedState,
		ModelState.UnknownState:                       UnknownState,
		ModelState.UnspecifiedStopState:               UnspecifiedStopState,
		ModelState.IdleState:                          IdleState,
		ModelState.OperatorInteractionState:           OperatorInteractionState,
		ModelState.MicrostopState:                     MicrostopState,
		ModelState.InletJamState:                      InletJamState,
		ModelState.OutletJamState:                     OutletJamState,
		ModelState.CongestionBypassState:              CongestionBypassState,
		ModelState.MissingBottleCapsRinneState:        MissingBottleCapsRinneState,
		ModelState.MissingBottleCapsUebergabeState:    MissingBottleCapsUebergabeState,
		ModelState.MaterialIssueOtherState:            MaterialIssueOtherState,
		ModelState.ChangeoverState:                    ChangeoverState,
		ModelState.ChangeoverPreparationState:         ChangeoverPreparationState,
		ModelState.ChangeoverPostprocessingState:      ChangeoverPostprocessingState,
		ModelState.CleaningState:                      CleaningState,
		ModelState.EmptyingState:                      EmptyingState,
		ModelState.SettingUpState:                     SettingUpState,
		ModelState.OperatorNotAtMachineState:          OperatorNotAtMachineState,
		ModelState.OperatorBreakState:                 OperatorBreakState,
		ModelState.NoShiftState:                       NoShiftState,
		ModelState.NoOrderState:                       NoOrderState,
		ModelState.EquipmentFailureState:              EquipmentFailureState,
		ModelState.EquipmentFailureStateWelder:        EquipmentFailureStateWelder,
		ModelState.EquipmentFailureStateExpender:      EquipmentFailureStateExpender,
		ModelState.EquipmentFailureStatePalletizer:    EquipmentFailureStatePalletizer,
		ModelState.EquipmentFailureStateUnderbody:     EquipmentFailureStateUnderbody,
		ModelState.EquipmentFailureStateTopcover:      EquipmentFailureStateTopcover,
		ModelState.ExternalFailureState:               ExternalFailureState,
		ModelState.ExternalInterferenceState:          ExternalInterferenceState,
		ModelState.CraneNotAvailableState:             CraneNotAvailableState,
		ModelState.PreventiveMaintenanceStop:          PreventiveMaintenanceStop,
		ModelState.TechnicalOtherStop:                 TechnicalOtherStop,
		ModelState.MaxState:                           MaxState,
	}

	value, ok := stateMap[stateString]
	if ok {
		return value
	}
	return 0
}
