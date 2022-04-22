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

func GetStateFromString(stateString string) uint64 {

	switch stateString {

	case ModelState.ProducingAtFullSpeedState:
		return ProducingAtFullSpeedState
	case ModelState.ProducingAtLowerThanFullSpeedState:
		return ProducingAtLowerThanFullSpeedState
	case ModelState.UnknownState:
		return UnknownState
	case ModelState.UnspecifiedStopState:
		return UnspecifiedStopState
	case ModelState.IdleState:
		return IdleState
	case ModelState.OperatorInteractionState:
		return OperatorInteractionState
	case ModelState.MicrostopState:
		return MicrostopState
	case ModelState.InletJamState:
		return InletJamState
	case ModelState.OutletJamState:
		return OutletJamState
	case ModelState.CongestionBypassState:
		return CongestionBypassState
	case ModelState.MissingBottleCapsRinneState:
		return MissingBottleCapsRinneState
	case ModelState.MissingBottleCapsUebergabeState:
		return MissingBottleCapsUebergabeState
	case ModelState.MaterialIssueOtherState:
		return MaterialIssueOtherState
	case ModelState.ChangeoverState:
		return ChangeoverState
	case ModelState.ChangeoverPreparationState:
		return ChangeoverPreparationState
	case ModelState.ChangeoverPostprocessingState:
		return ChangeoverPostprocessingState
	case ModelState.CleaningState:
		return CleaningState
	case ModelState.EmptyingState:
		return EmptyingState
	case ModelState.SettingUpState:
		return SettingUpState
	case ModelState.OperatorNotAtMachineState:
		return OperatorNotAtMachineState
	case ModelState.OperatorBreakState:
		return OperatorBreakState
	case ModelState.NoShiftState:
		return NoShiftState
	case ModelState.NoOrderState:
		return NoOrderState
	case ModelState.EquipmentFailureState:
		return EquipmentFailureState
	case ModelState.EquipmentFailureStateWelder:
		return EquipmentFailureStateWelder
	case ModelState.EquipmentFailureStateExpender:
		return EquipmentFailureStateExpender
	case ModelState.EquipmentFailureStatePalletizer:
		return EquipmentFailureStatePalletizer
	case ModelState.EquipmentFailureStateUnderbody:
		return EquipmentFailureStateUnderbody
	case ModelState.EquipmentFailureStateTopcover:
		return EquipmentFailureStateTopcover
	case ModelState.ExternalFailureState:
		return ExternalFailureState
	case ModelState.ExternalInterferenceState:
		return ExternalInterferenceState
	case ModelState.CraneNotAvailableState:
		return CraneNotAvailableState
	case ModelState.PreventiveMaintenanceStop:
		return PreventiveMaintenanceStop
	case ModelState.TechnicalOtherStop:
		return TechnicalOtherStop
	case ModelState.MaxState:
		return MaxState
	}

	return 0
}
