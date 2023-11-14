package worker

import (
	"github.com/stretchr/testify/assert"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"math/rand"
	"strings"
	"testing"
)

func TestRecreateTopic(t *testing.T) {
	valid := []string{
		"umh.v1.Chernobyl.MainSite.ReactorSection.Reactor1.ControlRoom.00-80-41-ae-fd-7e._historian.TemperatureSensor",
		"umh.v1.Chernobyl.MainSite.CoolingSystem.Pump2.FlowControl.34-67-89-bc-ef-01._historian.PressureGauge",
		"umh.v1.Chernobyl.BackupArea.EnergyStorage.BatteryPack3.ManagementUnit.10-20-30-40-50-60._historian.ChargeLevel",
		"umh.v1.Chernobyl.MainSite.ReactorSection.Reactor2.SafetySystem.20-40-60-80-a0-c0._historian.RadiationLevel",
		"umh.v1.Chernobyl.MainSite.TurbineHall.Turbine1.MonitoringStation.11-22-33-44-55-66._historian.VibrationAnalysis",
		"umh.v1.Chernobyl.Security.WatchTower1.Surveillance.Cam4.IP-192-168-1-4._historian.MotionDetection",
		"umh.v1.Chernobyl.VisitorCenter.LearningZone.InteractiveDisplay1.Device-77-88._historian.UserInteraction",
		"umh.v1.Chernobyl.MainSite.ElectricalRoom.Transformer1.VoltageRegulator.55-aa-bb-cc-dd-ee._historian.CurrentLoad",
		"umh.v1.Chernobyl.Auxiliary.Building3.FireSafety.SmokeDetector1.SN-998877._historian.SmokeAlarm",
		"umh.v1.Chernobyl.MainSite.Administration.NetworkServer.ServerRack2.MAC-aa-bb-cc-dd-ee._historian.DataTraffic",
		"umh.v1.Chernobyl.MainSite.CoolingSystem.Pump3.FlowControl.44-77-88-99-aa-bb._historian.WaterLevel",
		"umh.v1.Chernobyl.MainSite.ReactorSection.Reactor3.ControlRoom.22-44-66-88-aa-cc._historian.CoreTemperature",
		"umh.v1.Chernobyl.MainSite.EmergencyExit.SafetyLight1.SN-112233._historian.LightStatus",
		"umh.v1.Chernobyl.MainSite.ReactorSection.Reactor4.VentilationSystem.33-55-77-99-bb-dd._historian.AirQuality",
		"umh.v1.Chernobyl.Auxiliary.Building2.WaterSupply.MainValve1.ValveID-1234._historian.FlowRate",
		"umh.v1.Che-rnobyl.Auxiliary.Building2.WaterSupply.MainValve1.ValveID-1234._historian.FlowRate",
		"umh.v1.Che_rnobyl.Auxiliary.Building2.WaterSupply.MainValve1.ValveID-1234._historian.FlowRate",
		"umh.v1.Che_rnobyl.Auxiliary.Building2.WaterSupply.MainValve1.ValveID-1234._analytics.FlowRate",
	}
	invalid := []string{
		"umh.v1.Che_rnobyl.Auxiliary.Building2.WaterSupply.MainValve1.ValveID-1234._analytics.FlowRate.X",
		"umh.v1.Chernobyl.MainSite",
		"umh.v1.Chernobyl.Security",
		"umh.v1.Chernobyl.VisitorCenter",
		"umh.v1.Chernobyl.Auxiliary",
		"umh.v1.Chernobyl.MainSite.TurbineHall",
		"umh.v1.Chernobyl.MainSite.ReactorSection",
		"umh.v1.Chernobyl.MainSite.ElectricalRoom",
		"umh.v1.Chernobyl.MainSite.Administration",
		"umh.v1.Chernobyl.MainSite.CoolingSystem",
		"umh.v1.Chernobyl.BackupArea",
		"umh.v1.Chernobyl.MainSite.EmergencyExit",
		"umh.v1.Chernobyl.Security.Gate1",
		"umh.v1.Chernobyl.VisitorCenter.TicketCounter",
		"umh.v1.Chernobyl.Auxiliary.StorageRoom",
		"umh.v1.Chernobyl.MainSite.ControlCenter",
	}

	for _, validTopic := range valid {
		msg := shared.KafkaMessage{
			Headers:   nil,
			Topic:     validTopic,
			Key:       nil,
			Value:     nil,
			Offset:    0,
			Partition: 0,
		}
		_, err := recreateTopic(&msg)
		assert.NoError(t, err, "topic %s failed to parse", validTopic)
	}

	for _, invalidTopic := range invalid {
		msg := shared.KafkaMessage{
			Headers:   nil,
			Topic:     invalidTopic,
			Key:       nil,
			Value:     nil,
			Offset:    0,
			Partition: 0,
		}
		_, err := recreateTopic(&msg)
		assert.Errorf(t, err, "topic %s failed to parse", invalidTopic)
	}

	// Test with splits
	for _, validTopic := range valid {
		// Split the topic into parts
		parts := strings.Split(validTopic, ".")

		// Choose a random split point, ensuring it's not the first or the last part
		splitIndex := rand.Intn(len(parts)-1) + 1

		// Create a modified topic and key
		modifiedTopic := strings.Join(parts[:splitIndex], ".")
		key := "." + strings.Join(parts[splitIndex:], ".")

		msg := shared.KafkaMessage{
			Headers:   nil,
			Topic:     modifiedTopic,
			Key:       []byte(key),
			Value:     nil,
			Offset:    0,
			Partition: 0,
		}
		_, err := recreateTopic(&msg)
		assert.NoError(t, err, "modified topic %s with key %s failed to parse", modifiedTopic, key)
	}
}
