package datamodel

// ConvertOldToNew converts a state from the old data model to the new one
func ConvertOldToNew(oldState int) (newState int) {
	switch oldState {
	case 0:
		newState = ProducingAtFullSpeedState
	case 1:
		newState = UnspecifiedStopState
	case 2:
		newState = NoShiftState
	case 3:
		newState = TechnicalOtherStop
	case 4:
		newState = MaterialIssueOtherState
	case 5:
		newState = PreventiveMaintenanceStop
	case 6:
		newState = ChangeoverState
	case 7:
		newState = MicrostopState
	case 8:
		newState = OperatorBreakState
	case 9:
		newState = NoShiftState
	case 10:
		newState = InletJamState
	case 11:
		newState = NoOrderState
	case 12:
		newState = IdleState
	case 13:
		newState = OperatorInteractionState
	case 14:
		newState = ExternalInterferenceState
	case 15:
		newState = InletJamState
	case 16:
		newState = OutletJamState
	case 18:
		newState = CleaningState
	case 19:
		newState = ProducingAtLowerThanFullSpeedState
	case 1001:
		newState = MissingBottleCapsRinneState
	case 1002:
		newState = MissingBottleCapsUebergabeState
	default:
		newState = oldState
	}

	return newState
}

// ConvertNewToOld converts a state from the new data model to the old one (used to keep the old test files)
func ConvertNewToOld(newState int) (oldState int) {
	switch newState {
	case ProducingAtFullSpeedState:
		oldState = 0
	case UnspecifiedStopState:
		oldState = 1
	case NoOrderState:
		oldState = 2
	case TechnicalOtherStop:
		oldState = 3
	case MaterialIssueOtherState:
		oldState = 4
	case PreventiveMaintenanceStop:
		oldState = 5
	case ChangeoverState:
		oldState = 6
	case MicrostopState:
		oldState = 7
	case OperatorBreakState:
		oldState = 8
	case NoShiftState:
		oldState = 9
	// case InletJamState:
	//	oldState = 10
	// case NoOrderState:
	//	oldState = 11
	case IdleState:
		oldState = 12
	case OperatorInteractionState:
		oldState = 13
	case ExternalInterferenceState:
		oldState = 14
	case InletJamState:
		oldState = 15
	case OutletJamState:
		oldState = 16
	case CleaningState:
		oldState = 18
	case ProducingAtLowerThanFullSpeedState:
		oldState = 19
	case MissingBottleCapsRinneState:
		oldState = 1001
	case MissingBottleCapsUebergabeState:
		oldState = 1002
	default:
		oldState = newState
	}

	return oldState
}
