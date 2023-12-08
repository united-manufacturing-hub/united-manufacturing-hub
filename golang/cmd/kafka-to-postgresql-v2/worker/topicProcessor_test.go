package worker

import (
	"github.com/stretchr/testify/assert"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"math/rand"
	"strings"
	"testing"
)

func TestRecreateTopicSimple(t *testing.T) {
	a := "umh.v1.Chernobyl.VisitorCenter._historian"
	b := "umh.v1.Chernobyl.MainSite.ReactorSection.Reactor1.ControlRoom.00-80-41-ae-fd-7e._historian.TemperatureSensor"
	c := "umh.v1.Chernobyl.VisitorCenter._historian.abc"
	d := "umh.v1.Chernobyl.VisitorCenter._historian.abc.def"

	// a
	// Enterprise: Chernobyl
	// Site: VisitorCenter
	// Area:
	// ProductionLine:
	// WorkCell:
	// OriginId:
	// Usecase: historian
	// Tag:

	// b
	// Enterprise: Chernobyl
	// Site: MainSite
	// Area: ReactorSection
	// ProductionLine: Reactor1
	// WorkCell: ControlRoom
	// OriginId: 00-80-41-ae-fd-7e
	// Usecase: historian
	// Tag: TemperatureSensor

	// c
	// Enterprise: Chernobyl
	// Site: VisitorCenter
	// Area:
	// ProductionLine:
	// WorkCell:
	// OriginId:
	// Usecase: historian
	// Tag: abc

	// d
	// Enterprise: Chernobyl
	// Site: VisitorCenter
	// Area:
	// ProductionLine:
	// WorkCell:
	// OriginId:
	// Usecase: historian
	// Tag: abc.def

	msgA := shared.KafkaMessage{
		Headers:   nil,
		Topic:     a,
		Key:       nil,
		Value:     nil,
		Offset:    0,
		Partition: 0,
	}
	msgB := shared.KafkaMessage{
		Headers:   nil,
		Topic:     b,
		Key:       nil,
		Value:     nil,
		Offset:    0,
		Partition: 0,
	}
	msgC := shared.KafkaMessage{
		Headers:   nil,
		Topic:     c,
		Key:       nil,
		Value:     nil,
		Offset:    0,
		Partition: 0,
	}
	msgD := shared.KafkaMessage{
		Headers:   nil,
		Topic:     d,
		Key:       nil,
		Value:     nil,
		Offset:    0,
		Partition: 0,
	}

	topicA, err := recreateTopicREGEX(&msgA)
	assert.NoError(t, err)
	assert.Equal(t, "Chernobyl", topicA.Enterprise)
	assert.Equal(t, "VisitorCenter", topicA.Site)
	assert.Equal(t, "", topicA.Area)
	assert.Equal(t, "", topicA.ProductionLine)
	assert.Equal(t, "", topicA.WorkCell)
	assert.Equal(t, "", topicA.OriginId)
	assert.Equal(t, "historian", topicA.Usecase)
	assert.Equal(t, "", topicA.Tag)

	topicB, err := recreateTopicREGEX(&msgB)
	assert.NoError(t, err)
	assert.Equal(t, "Chernobyl", topicB.Enterprise)
	assert.Equal(t, "MainSite", topicB.Site)
	assert.Equal(t, "ReactorSection", topicB.Area)
	assert.Equal(t, "Reactor1", topicB.ProductionLine)
	assert.Equal(t, "ControlRoom", topicB.WorkCell)
	assert.Equal(t, "00-80-41-ae-fd-7e", topicB.OriginId)
	assert.Equal(t, "historian", topicB.Usecase)
	assert.Equal(t, "TemperatureSensor", topicB.Tag)

	topicC, err := recreateTopicREGEX(&msgC)
	assert.NoError(t, err)
	assert.Equal(t, "Chernobyl", topicC.Enterprise)
	assert.Equal(t, "VisitorCenter", topicC.Site)
	assert.Equal(t, "", topicC.Area)
	assert.Equal(t, "", topicC.ProductionLine)
	assert.Equal(t, "", topicC.WorkCell)
	assert.Equal(t, "", topicC.OriginId)
	assert.Equal(t, "historian", topicC.Usecase)
	assert.Equal(t, "abc", topicC.Tag)

	topicD, err := recreateTopicREGEX(&msgD)
	assert.NoError(t, err)
	assert.Equal(t, "Chernobyl", topicD.Enterprise)
	assert.Equal(t, "VisitorCenter", topicD.Site)
	assert.Equal(t, "", topicD.Area)
	assert.Equal(t, "", topicD.ProductionLine)
	assert.Equal(t, "", topicD.WorkCell)
	assert.Equal(t, "", topicD.OriginId)
	assert.Equal(t, "historian", topicD.Usecase)
	assert.Equal(t, "abc.def", topicD.Tag)

	// alt implementation
	topicA, err = recreateTopic(&msgA)
	assert.NoError(t, err)
	assert.Equal(t, "Chernobyl", topicA.Enterprise)
	assert.Equal(t, "VisitorCenter", topicA.Site)
	assert.Equal(t, "", topicA.Area)
	assert.Equal(t, "", topicA.ProductionLine)
	assert.Equal(t, "", topicA.WorkCell)
	assert.Equal(t, "", topicA.OriginId)
	assert.Equal(t, "historian", topicA.Usecase)
	assert.Equal(t, "", topicA.Tag)

	topicB, err = recreateTopic(&msgB)
	assert.NoError(t, err)
	assert.Equal(t, "Chernobyl", topicB.Enterprise)
	assert.Equal(t, "MainSite", topicB.Site)
	assert.Equal(t, "ReactorSection", topicB.Area)
	assert.Equal(t, "Reactor1", topicB.ProductionLine)
	assert.Equal(t, "ControlRoom", topicB.WorkCell)
	assert.Equal(t, "00-80-41-ae-fd-7e", topicB.OriginId)
	assert.Equal(t, "historian", topicB.Usecase)
	assert.Equal(t, "TemperatureSensor", topicB.Tag)

	topicC, err = recreateTopic(&msgC)
	assert.NoError(t, err)
	assert.Equal(t, "Chernobyl", topicC.Enterprise)
	assert.Equal(t, "VisitorCenter", topicC.Site)
	assert.Equal(t, "", topicC.Area)
	assert.Equal(t, "", topicC.ProductionLine)
	assert.Equal(t, "", topicC.WorkCell)
	assert.Equal(t, "", topicC.OriginId)
	assert.Equal(t, "historian", topicC.Usecase)
	assert.Equal(t, "abc", topicC.Tag)

	topicD, err = recreateTopic(&msgD)
	assert.NoError(t, err)
	assert.Equal(t, "Chernobyl", topicD.Enterprise)
	assert.Equal(t, "VisitorCenter", topicD.Site)
	assert.Equal(t, "", topicD.Area)
	assert.Equal(t, "", topicD.ProductionLine)
	assert.Equal(t, "", topicD.WorkCell)
	assert.Equal(t, "", topicD.OriginId)
	assert.Equal(t, "historian", topicD.Usecase)
	assert.Equal(t, "abc.def", topicD.Tag)

}

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
		"umh.v1.Chernobyl.VisitorCenter._historian",
		"umh.v1.Che_rnobyl.Auxiliary.Building2.WaterSupply.MainValve1.ValveID-1234._analytics.FlowRate.X",
	}
	invalid := []string{
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

	var validMsgs []shared.KafkaMessage
	var invalidMsgs []shared.KafkaMessage

	for _, validTopic := range valid {
		msg := shared.KafkaMessage{
			Headers:   nil,
			Topic:     validTopic,
			Key:       nil,
			Value:     nil,
			Offset:    0,
			Partition: 0,
		}
		validMsgs = append(validMsgs, msg)
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
		invalidMsgs = append(invalidMsgs, msg)
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
		validMsgs = append(validMsgs, msg)
	}

	t.Run("regex", func(t *testing.T) {
		for _, msg := range validMsgs {
			m := msg
			_, err := recreateTopicREGEX(&m)
			assert.NoError(t, err)
		}
		for _, msg := range invalidMsgs {
			m := msg
			_, err := recreateTopicREGEX(&m)
			assert.Errorf(t, err, "Topic %s should be invalid", msg.Topic)
		}
	})
	t.Run("alt", func(t *testing.T) {
		for _, msg := range validMsgs {
			m := msg
			_, err := recreateTopic(&m)
			assert.NoError(t, err)
		}
		for _, msg := range invalidMsgs {
			m := msg
			d, err := recreateTopic(&m)
			assert.Errorf(t, err, "Topic %s should be invalid: %+v", msg.Topic, d)
		}
	})
}

func BenchmarkRecreateTopic(b *testing.B) {
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
		"umh.v1.Che_rnobyl.Auxiliary.Building2.WaterSupply.MainValve1.ValveID-1234._analytics.FlowRate.X",
	}
	var msgs []shared.KafkaMessage
	for _, validTopic := range valid {
		msg := shared.KafkaMessage{
			Headers:   nil,
			Topic:     validTopic,
			Key:       nil,
			Value:     nil,
			Offset:    0,
			Partition: 0,
		}
		msgs = append(msgs, msg)
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
		msgs = append(msgs, msg)
	}
	b.Run("regex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, msg := range msgs {
				m := msg
				_, err := recreateTopicREGEX(&m)
				if err != nil {
					b.Error(err)
				}
			}
		}
	})

	b.Run("alt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, msg := range msgs {
				m := msg
				_, err := recreateTopic(&m)
				if err != nil {
					b.Error(err)
				}
			}
		}
	})
}
