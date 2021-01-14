package datamodel

// ConvertOldToNew converts a state from the old data model to the new one
func ConvertOldToNew(OldState int) (NewState int) {
	switch OldState {
	case 0:
		NewState = ProducingAtFullSpeedState
	case 1:
		NewState = UnspecifiedStopState
	case 2:
		NewState = NoShiftState
	case 3:
		NewState = TechnicalOtherStop
	case 4:
		NewState = MaterialIssueOtherState
	case 5:
		NewState = PreventiveMaintenanceStop
	case 6:
		NewState = ChangeoverState
	case 7:
		NewState = MicrostopState
	case 8:
		NewState = OperatorBreakState
	case 9:
		NewState = NoShiftState
	case 10:
		NewState = InletJamState
	case 11:
		NewState = NoOrderState
	case 12:
		NewState = IdleState
	case 13:
		NewState = OperatorInteractionState
	case 14:
		NewState = ExternalInterferenceState
	case 15:
		NewState = InletJamState
	case 16:
		NewState = OutletJamState
	case 18:
		NewState = CleaningState
	case 19:
		NewState = ProducingAtLowerThanFullSpeedState
	case 1001:
		NewState = MissingBottleCapsRinneState
	case 1002:
		NewState = MissingBottleCapsUebergabeState
	default:
		NewState = OldState
	}

	return
}

// ConvertNewToOld converts a state from the new data model to the old one (used to keep the old test files)
func ConvertNewToOld(NewState int) (OldState int) {
	switch NewState {
	case ProducingAtFullSpeedState:
		OldState = 0
	case UnspecifiedStopState:
		OldState = 1
	case NoOrderState:
		OldState = 2
	case TechnicalOtherStop:
		OldState = 3
	case MaterialIssueOtherState:
		OldState = 4
	case PreventiveMaintenanceStop:
		OldState = 5
	case ChangeoverState:
		OldState = 6
	case MicrostopState:
		OldState = 7
	case OperatorBreakState:
		OldState = 8
	case NoShiftState:
		OldState = 9
	//case InletJamState:
	//	OldState = 10
	//case NoOrderState:
	//	OldState = 11
	case IdleState:
		OldState = 12
	case OperatorInteractionState:
		OldState = 13
	case ExternalInterferenceState:
		OldState = 14
	case InletJamState:
		OldState = 15
	case OutletJamState:
		OldState = 16
	case CleaningState:
		OldState = 18
	case ProducingAtLowerThanFullSpeedState:
		OldState = 19
	case MissingBottleCapsRinneState:
		OldState = 1001
	case MissingBottleCapsUebergabeState:
		OldState = 1002
	default:
		OldState = NewState
	}

	return
}
