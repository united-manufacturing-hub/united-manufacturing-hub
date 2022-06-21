package datamodel

import (
	"fmt"
	"go.uber.org/zap"
)

type LanguageCode int

const (
	LanguageGerman  LanguageCode = 0
	LanguageEnglish LanguageCode = 1
)

// ConvertStateToString converts a state in integer format to a human readable string
func ConvertStateToString(state int, languageCode LanguageCode) (stateString string) {
	if int(languageCode) > 1 || int(languageCode) < 1 {
		zap.S().Warnf("Invalid language code %d, defaulting to english", languageCode)
	}
	if languageCode == 0 { // GERMAN
		switch state {
		case ProducingAtFullSpeedState:
			stateString = "Maschine läuft"
		case ProducingAtLowerThanFullSpeedState:
			stateString = "Maschine läuft mit verringerter Geschwindigkeit"
		case UnknownState:
			stateString = "Keine Daten"
		case IdleState:
			stateString = "Bereit"
		case OperatorInteractionState:
			stateString = "Bedienereingriff"
		case UnspecifiedStopState:
			stateString = "Unbekannter Stopp"
		case MicrostopState:
			stateString = "Mikrostopp"
		case InletJamState:
			stateString = "Mangel am Einlauf"
		case OutletJamState:
			stateString = "Mangel am Auslauf"
		case CongestionBypassState:
			stateString = "Mangel an Hilfsmaterialien"
		case MissingBottleCapsRinneState:
			stateString = "Mangel an Kronkorken (Rinne)"
		case MissingBottleCapsUebergabeState:
			stateString = "Mangel an Kronkorken (Übergabe)"
		case MaterialIssueOtherState:
			stateString = "Sonstige Materialprobleme"
		case ChangeoverState:
			stateString = "Rüsten"
		case ChangeoverPreparationState:
			stateString = "Vorbereitung"
		case ChangeoverPostprocessingState:
			stateString = "Nachbereitung"
		case CleaningState:
			stateString = "Reinigen"
		case EmptyingState:
			stateString = "Leeren"
		case SettingUpState:
			stateString = "Vorbereiten"
		case OperatorNotAtMachineState:
			stateString = "Maschinenbediener fehlt"
		case OperatorBreakState:
			stateString = "Pause"
		case NoShiftState:
			stateString = "Keine Schicht"
		case NoOrderState:
			stateString = "Kein Auftrag"
		case EquipmentFailureState:
			stateString = "Maschinenstörung"
		case EquipmentFailureStateWelder:
			stateString = "Maschinenstörung Schweißer"
		case EquipmentFailureStateExpender:
			stateString = "Maschinenstörung Spreizer"
		case EquipmentFailureStatePalletizer:
			stateString = "Maschinenstörung Palettierer"
		case EquipmentFailureStateUnderbody:
			stateString = "Maschinenstörung Unterboden"
		case EquipmentFailureStateTopcover:
			stateString = "Maschinenstörung Oberboden"
		case ExternalFailureState:
			stateString = "Externe Störung"
		case ExternalInterferenceState:
			stateString = "Sonstige externe Störung"
		case CraneNotAvailableState:
			stateString = "Kran nicht verfügbar"
		case PreventiveMaintenanceStop:
			stateString = "Wartung"
		case TechnicalOtherStop:
			stateString = "Sonstige technische Störung"
		default:
			stateString = fmt.Sprintf("Unbekannter Zustand %d", state)
		}
	} else { //ENGLISH
		switch state {
		case ProducingAtFullSpeedState:
			stateString = "Producing"
		case ProducingAtLowerThanFullSpeedState:
			stateString = "Producing at lower than set speed"
		case UnknownState:
			stateString = "No data"
		case IdleState:
			stateString = "Idle"
		case OperatorInteractionState:
			stateString = "Operator interaction"
		case UnspecifiedStopState:
			stateString = "Unknown stop"
		case MicrostopState:
			stateString = "Microstop"
		case InletJamState:
			stateString = "Inlet jam"
		case OutletJamState:
			stateString = "Outlet jam"
		case CongestionBypassState:
			stateString = "Congestion in the bypass flow"
		case MaterialIssueOtherState:
			stateString = "Other material issues"
		case ChangeoverState:
			stateString = "Changeover"
		case ChangeoverPreparationState:
			stateString = "Preparation"
		case ChangeoverPostprocessingState:
			stateString = "Postprocessing"
		case CleaningState:
			stateString = "Cleaning"
		case EmptyingState:
			stateString = "Emptying"
		case SettingUpState:
			stateString = "Setting up"
		case OperatorNotAtMachineState:
			stateString = "Operator missing"
		case OperatorBreakState:
			stateString = "Break"
		case NoShiftState:
			stateString = "No shift"
		case NoOrderState:
			stateString = "No order"
		case EquipmentFailureState:
			stateString = "Equipment failure"
		case EquipmentFailureStateWelder:
			stateString = "Equipment failure welder"
		case EquipmentFailureStateExpender:
			stateString = "Equipment failure expender"
		case EquipmentFailureStatePalletizer:
			stateString = "Equipment failure palletizer"
		case EquipmentFailureStateUnderbody:
			stateString = "Equipment failure underbody"
		case EquipmentFailureStateTopcover:
			stateString = "Equipment failure topcover"
		case ExternalFailureState:
			stateString = "External failure"
		case ExternalInterferenceState:
			stateString = "External interference"
		case CraneNotAvailableState:
			stateString = "Crane not available"
		case PreventiveMaintenanceStop:
			stateString = "Maintenance"
		case TechnicalOtherStop:
			stateString = "Other technical issue"
		default:
			stateString = fmt.Sprintf("Unknown state %d", state)
		}
	}

	return
}
