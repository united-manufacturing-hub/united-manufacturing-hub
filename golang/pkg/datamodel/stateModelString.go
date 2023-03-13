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

import (
	"fmt"
	"go.uber.org/zap"
)

type LanguageCode int

const (
	LanguageGerman  LanguageCode = 0
	LanguageEnglish LanguageCode = 1
	LanguageTurkish LanguageCode = 2
)

// ConvertStateToString converts a state in integer format to a human-readable string
func ConvertStateToString(state int, languageCode LanguageCode) (stateString string) {

	switch languageCode {
	case LanguageGerman:
		stateString = stateToGerman(state)
	case LanguageEnglish:
		stateString = stateToEnglish(state)
	case LanguageTurkish:
		stateString = stateToTurkish(state)
	default:
		zap.S().Warnf("Invalid language code %d, defaulting to english", languageCode)
		stateString = stateToEnglish(state)
	}

	return stateString
}

func stateToEnglish(state int) (stateString string) {
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
	return stateString
}

func stateToTurkish(state int) (stateString string) {
	switch state {
	case ProducingAtFullSpeedState:
		stateString = "Makine Çalışıyor"
	case ProducingAtLowerThanFullSpeedState:
		stateString = "Makine Düşük Hızda Çalışıyor"
	case UnknownState:
		stateString = "Veri Yok"
	case IdleState:
		stateString = "Hazır"
	case OperatorInteractionState:
		stateString = "Operatör Müdahalesi"
	case UnspecifiedStopState:
		stateString = "Bilinmeyen Duruş"
	case MicrostopState:
		stateString = "Kısa Duruş"
	case InletJamState:
		stateString = "Mangel am Einlauf"
	case OutletJamState:
		stateString = "Mangel am Auslauf"
	case CongestionBypassState:
		stateString = "Yarı Mamül Eksikliği"
	case MissingBottleCapsRinneState:
		stateString = "Mangel an Kronkorken (Rinne)"
	case MissingBottleCapsUebergabeState:
		stateString = "Mangel an Kronkorken (Übergabe)"
	case MaterialIssueOtherState:
		stateString = "Diğer Hammadde Sorunları"
	case ChangeoverState:
		stateString = "Model Değişimi"
	case ChangeoverPreparationState:
		stateString = "Model Değişim Hazırlık"
	case ChangeoverPostprocessingState:
		stateString = "Model Değişim Son Kontrol"
	case CleaningState:
		stateString = "Temizlik"
	case EmptyingState:
		stateString = "Boşaltma"
	case SettingUpState:
		stateString = "Hazırlama"
	case OperatorNotAtMachineState:
		stateString = "Operatör Makinede Değil"
	case OperatorBreakState:
		stateString = "Mola"
	case NoShiftState:
		stateString = "Vardiya Yok"
	case NoOrderState:
		stateString = "Sipariş Yok"
	case EquipmentFailureState:
		stateString = "Makine Arızası"
	case EquipmentFailureStateWelder:
		stateString = "Makine Arızası - Kaynak"
	case EquipmentFailureStateExpender:
		stateString = "Makine Arızası - Tutucu"
	case EquipmentFailureStatePalletizer:
		stateString = "Makine Arızası - Palet"
	case EquipmentFailureStateUnderbody:
		stateString = "Makine Arızası - Gövde"
	case EquipmentFailureStateTopcover:
		stateString = "Makine Arızası - Üst Taraf"
	case ExternalFailureState:
		stateString = "Harici Arıza"
	case ExternalInterferenceState:
		stateString = "Dış Müdahale"
	case CraneNotAvailableState:
		stateString = "Vinç Mevcut Değil"
	case PreventiveMaintenanceStop:
		stateString = "Bakım"
	case TechnicalOtherStop:
		stateString = "Diğer Teknik Arızalar"
	default:
		stateString = fmt.Sprintf("Bilinmeyen Durum %d", state)
	}
	return stateString
}

func stateToGerman(state int) (stateString string) {
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
	return stateString
}
