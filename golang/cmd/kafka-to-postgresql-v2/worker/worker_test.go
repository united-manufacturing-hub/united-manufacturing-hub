package worker

import (
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/helper"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/postgresql"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"math/rand"
	"strings"
	"testing"
	"time"
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
		"umh.v1.Che_rnobyl.Auxiliary.Building2.WaterSupply.MainValve1.ValveID-1234._analytics.FlowRate.X",
		"umh.v1.Che_rnobyl.Auxiliary.Building2.WaterSupply.MainValve1.ValveID-1234._analytics.FlowRate.X.Y.Z",
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
func TestParseHistorianPayload(t *testing.T) {
	sV := "this is a string"
	var iV float32 = 1
	var fV float32 = 1.5
	var bV float32 = 1.0
	testCases := []struct {
		name string
		// input is the JSON payload to parse
		input []byte
		// tag is the tag parsed from the topic, including eventual tag groups
		tag      string
		expected []sharedStructs.HistorianValue
		wantErr  bool
	}{
		{
			name: "String value",
			input: []byte(`{
				"timestamp_ms": 12345,
				"stringValue": "this is a string"
			}`),
			tag: "tag1",
			expected: []sharedStructs.HistorianValue{
				{
					Name:        "tag1$stringValue",
					StringValue: &sV,
					IsNumeric:   false,
				},
			},
			wantErr: false,
		},
		{
			name: "Int value",
			input: []byte(`{
				"timestamp_ms": 12345,
				"intValue": 1
			}`),
			tag: "tag2",
			expected: []sharedStructs.HistorianValue{
				{
					Name:         "tag2$intValue",
					NumericValue: &iV,
					IsNumeric:    true,
				},
			},
			wantErr: false,
		},
		{
			name: "Float value",
			input: []byte(`{
				"timestamp_ms": 12345,
				"floatValue": 1.5
			}`),
			tag: "tag3",
			expected: []sharedStructs.HistorianValue{
				{
					Name:         "tag3$floatValue",
					NumericValue: &fV,
					IsNumeric:    true,
				},
			},
			wantErr: false,
		},
		{
			name: "Bool value",
			input: []byte(`{
				"timestamp_ms": 12345,
				"boolValue": true
			}`),
			tag: "tag4",
			expected: []sharedStructs.HistorianValue{
				{
					Name:         "tag4$boolValue",
					NumericValue: &bV,
					IsNumeric:    true,
				},
			},
			wantErr: false,
		},
		{
			name: "Nested struct value",
			input: []byte(`{
				"timestamp_ms": 12345,
				"structValue": {
					"stringValue": "this is a string",
					"intValue": 1,
					"floatValue": 1.5,
					"boolValue": true
				}
			}`),
			tag: "tag5",
			expected: []sharedStructs.HistorianValue{
				{
					Name:        "tag5$structValue$stringValue",
					StringValue: &sV,
					IsNumeric:   false,
				},
				{
					Name:         "tag5$structValue$intValue",
					NumericValue: &iV,
					IsNumeric:    true,
				},
				{
					Name:         "tag5$structValue$floatValue",
					NumericValue: &fV,
					IsNumeric:    true,
				},
				{
					Name:         "tag5$structValue$boolValue",
					NumericValue: &bV,
					IsNumeric:    true,
				},
			},
			wantErr: false,
		},
		{
			name: "Unsupported type",
			input: []byte(`{
				"timestamp_ms": 12345,
				"unsupportedValue": ["this", "is", "an", "array"]
			}`),
			tag:      "tag6",
			expected: []sharedStructs.HistorianValue{},
			wantErr:  true,
		},
		{
			name: "Duplicate tag group",
			input: []byte(`{
				"timestamp_ms": 12345,
				"duplicateTag": "this is a string"
			}`),
			tag: "duplicateTag",
			expected: []sharedStructs.HistorianValue{
				{
					Name:        "duplicateTag",
					StringValue: &sV,
					IsNumeric:   false,
				},
			},
			wantErr: false,
		},
		{
			name: "Multiple tag groups from topic",
			input: []byte(`{
				"timestamp_ms": 12345,
				"multipleTagGroups": "this is a string"
			}`),
			tag: "multipleTagGroups$tag1$tag2$tag3",
			expected: []sharedStructs.HistorianValue{
				{
					Name:        "multipleTagGroups$tag1$tag2$tag3$multipleTagGroups",
					StringValue: &sV,
					IsNumeric:   false,
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid JSON",
			input: []byte(`{
				"timestamp_ms": 12345,
			}`),
			tag:      "tag7",
			expected: []sharedStructs.HistorianValue{},
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			values, timestampMs, err := parseHistorianPayload(tc.input, tc.tag)
			assert.Equal(t, tc.wantErr, err != nil, "unexpected error. want %v, got %v", tc.wantErr, err)
			assert.ElementsMatch(t, tc.expected, values, "unexpected values. want %+v, got %+v", tc.expected, values)
			if !tc.wantErr {
				assert.Equal(t, int64(12345), timestampMs, "unexpected timestamp. want %d, got %d", 12345, timestampMs)
			}
		})
	}
}

func TestHandleParsing(t *testing.T) {
	helper.InitTestLogging()
	senderChannel := make(chan *shared.KafkaMessage, 100)
	kafkaClient := kafka.GetMockKafkaClient(t, senderChannel)
	postgresqlClient := postgresql.CreateMockConnection(t)

	mock, ok := postgresqlClient.Db.(pgxmock.PgxPoolIface)
	assert.True(t, ok)

	msg := shared.KafkaMessage{
		Headers: nil,
		Topic:   "umh.v1.abc._analytics",
		Key:     []byte("work-order.create"),
		Value: []byte(`
{
   "external_work_order_id":"#1244",
   "product":{
      "external_product_id":"1234",
      "cycle_time_ms":1000
   },
   "quantity":100
}
`),
		Offset:    0,
		Partition: 0,
	}
	senderChannel <- &msg

	// Expect Query from GetOrInsertAsset
	mock.ExpectQuery(`SELECT id FROM asset WHERE enterprise = \$1 AND site = \$2 AND area = \$3 AND line = \$4 AND workcell = \$5 AND origin_id = \$6`).
		WithArgs("abc", "", "", "", "", "").
		WillReturnRows(mock.NewRows([]string{"id"}).AddRow(1))

	// Expect Query from GetOrInsertProduct (assume product type exists)
	mock.ExpectQuery(`SELECT product_type_id FROM product_type WHERE external_product_type_id = \$1 AND asset_id = \$2`).
		WithArgs("1234", 1).
		WillReturnRows(mock.NewRows([]string{"product_type_id"}).AddRow(1))
	// Expect Exec from InsertWorkOrderCreate
	mock.ExpectBeginTx(pgx.TxOptions{})
	mock.ExpectExec(`
		INSERT INTO work_order\(external_work_order_id, asset_id, product_type_id, quantity, status, start_time, end_time\) VALUES \(\$1, \$2, \$3, \$4, \$5, CASE WHEN \$6 IS NOT NULL THEN to_timestamp\(\$6/1000\) END, CASE WHEN \$7 IS NOT NULL THEN to_timestamp\(\$7/1000\) END\)
	`).WithArgs("#1244", 1, 1, 100, 0, helper.MatchInt64Ptr(nil), helper.MatchInt64Ptr(nil)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	msgChannel := kafkaClient.GetMessages()
	go handleParsing(msgChannel, 0, kafkaClient, postgresqlClient)

	time.Sleep(1 * time.Second)

	// Validate that the message was marked as processed
	assert.Equal(t, 0, len(msgChannel), "unexpected number of messages in the channel. want %d, got %d", 0, len(msgChannel))
	assert.Equal(t, uint64(1), kafkaClient.GetMarkedMessageCount(), "unexpected number of marked messages. want %d, got %d", 1, kafkaClient.GetMarkedMessageCount())
}
