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
	"testing"
	"time"
)

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
	INSERT INTO work_order
            \(external_work_order_id,
             asset_id,
             product_type_id,
             quantity,
             status,
             start_time,
             end_time\)
		VALUES      \(\$1,
					 \$2,
					 \$3,
					 \$4,
					 \$5,
					 CASE
					   WHEN \$6\:\:BIGINT IS NOT NULL THEN to_timestamp\(\$6\:\:BIGINT \/ 1000.0\)
					   ELSE NULL
					 END \:\: timestamptz,
					 CASE
					   WHEN \$7\:\:BIGINT IS NOT NULL THEN to_timestamp\(\$7\:\:BIGINT \/ 1000.0\)
					   ELSE NULL
					 END \:\: timestamptz\) 
	`).WithArgs("#1244", 1, 1, 100, 0, helper.Uint64PtrToNullInt64(nil), helper.Uint64PtrToNullInt64(nil)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	msgChannel := kafkaClient.GetMessages()
	go handleParsing(msgChannel, 0, kafkaClient, postgresqlClient)

	time.Sleep(1 * time.Second)

	// Validate that the message was marked as processed
	assert.Equal(t, 0, len(msgChannel), "unexpected number of messages in the channel. want %d, got %d", 0, len(msgChannel))
	assert.Equal(t, uint64(1), kafkaClient.GetMarkedMessageCount(), "unexpected number of marked messages. want %d, got %d", 1, kafkaClient.GetMarkedMessageCount())
}
