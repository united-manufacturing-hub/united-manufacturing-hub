// This file was generated from JSON Schema using quicktype, do not modify it directly.
// To parse and unparse this JSON data, add this code to your project and do:
//
//    statusMessage, err := UnmarshalStatusMessage(bytes)
//    bytes, err = statusMessage.Marshal()

package generated

import "bytes"
import "errors"

import "encoding/json"

func UnmarshalStatusMessage(data []byte) (StatusMessage, error) {
	var r StatusMessage
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *StatusMessage) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

// Schema for the status message containing system state and DFC information
type StatusMessage struct {
	Core    Core                   `json:"core"`
	General General                `json:"general"`
	Plugins map[string]interface{} `json:"plugins"`
}

type Core struct {
	Agent             Agent                  `json:"agent"`
	Container         Container              `json:"container"`
	Dfcs              []Dfc                  `json:"dfcs"`
	EventsTable       map[string]EventsTable `json:"eventsTable,omitempty"`
	Latency           Latency                `json:"latency"`
	Redpanda          Redpanda               `json:"redpanda"`
	ReleaseChannel    string                 `json:"releaseChannel"`
	SupportedFeatures []string               `json:"supportedFeatures"`
	UnsTable          map[string]UnsTable    `json:"unsTable,omitempty"`
	Version           string                 `json:"version"`
}

type Agent struct {
	Health Health `json:"health"`
}

type Health struct {
	Message string `json:"message"`
	State   string `json:"state"`
}

type Container struct {
	Architecture string `json:"architecture"`
	CPU          CPU    `json:"cpu"`
	Disk         Disk   `json:"disk"`
	Hwid         string `json:"hwid"`
	Memory       Memory `json:"memory"`
}

type CPU struct {
	Health Health  `json:"health"`
	Limit  float64 `json:"limit"`
	Usage  float64 `json:"usage"`
}

type Disk struct {
	Health Health  `json:"health"`
	Limit  float64 `json:"limit"`
	Usage  float64 `json:"usage"`
}

type Memory struct {
	Health Health  `json:"health"`
	Limit  float64 `json:"limit"`
	Usage  float64 `json:"usage"`
}

type Dfc struct {
	Connections        []Connection `json:"connections,omitempty"`
	CurrentVersionUUID *string      `json:"currentVersionUUID"`
	DeploySuccess      bool         `json:"deploySuccess"`
	Health             *Health      `json:"health"`
	Metrics            *DFCMetrics  `json:"metrics"`
	Name               *string      `json:"name"`
	Type               Type         `json:"type"`
	UUID               string       `json:"uuid"`
	DataContract       *string      `json:"dataContract,omitempty"`
	InputType          *string      `json:"inputType,omitempty"`
	IsReadOnly         *bool        `json:"isReadOnly,omitempty"`
	OutputType         *string      `json:"outputType,omitempty"`
}

type Connection struct {
	Health  Health  `json:"health"`
	Host    string  `json:"host"`
	Latency float64 `json:"latency"`
	Name    string  `json:"name"`
	Port    float64 `json:"port"`
	UUID    string  `json:"uuid"`
}

type DFCMetrics struct {
	FailedMessages   float64 `json:"failedMessages"`
	Throughput       float64 `json:"throughput"`
	Unprocessable    float64 `json:"unprocessable"`
	Unprocessable24H float64 `json:"unprocessable24h"`
}

type EventsTable struct {
	Bridges         []string    `json:"bridges"`
	Error           string      `json:"error"`
	Origin          *string     `json:"origin"`
	RawKafkaMessage EventKafka  `json:"rawKafkaMessage"`
	TimestampMS     float64     `json:"timestamp_ms"`
	UnsTreeID       string      `json:"unsTreeId"`
	Value           *EventValue `json:"value"`
}

type EventKafka struct {
	Destination            []string          `json:"destination"`
	Headers                map[string]string `json:"headers"`
	KafkaInsertedTimestamp float64           `json:"kafkaInsertedTimestamp"`
	Key                    string            `json:"key"`
	LastPayload            string            `json:"lastPayload"`
	MessagesPerMinute      float64           `json:"messagesPerMinute"`
	Origin                 []string          `json:"origin"`
	Topic                  string            `json:"topic"`
}

type Latency struct {
	Avg float64 `json:"avg"`
	Max float64 `json:"max"`
	Min float64 `json:"min"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
}

type Redpanda struct {
	Health     Health  `json:"health"`
	Throughput float64 `json:"throughput"`
}

type UnsTable struct {
	Area       *string  `json:"area,omitempty"`
	Enterprise string   `json:"enterprise"`
	EventGroup *string  `json:"eventGroup,omitempty"`
	EventTag   *string  `json:"eventTag,omitempty"`
	Instances  []string `json:"instances,omitempty"`
	IsError    *bool    `json:"isError,omitempty"`
	Line       *string  `json:"line,omitempty"`
	OriginID   *string  `json:"originId,omitempty"`
	Schema     *string  `json:"schema,omitempty"`
	Site       *string  `json:"site,omitempty"`
	WorkCell   *string  `json:"workCell,omitempty"`
}

type General struct {
	Location map[string]string `json:"location"`
}

type Type string

const (
	Custom            Type = "custom"
	DataBridge        Type = "data-bridge"
	ProtocolConverter Type = "protocol-converter"
	Uninitialized     Type = "uninitialized"
)

type EventValue struct {
	Bool   *bool
	Double *float64
	String *string
}

func (x *EventValue) UnmarshalJSON(data []byte) error {
	object, err := unmarshalUnion(data, nil, &x.Double, &x.Bool, &x.String, false, nil, false, nil, false, nil, false, nil, true)
	if err != nil {
		return err
	}
	if object {
	}
	return nil
}

func (x *EventValue) MarshalJSON() ([]byte, error) {
	return marshalUnion(nil, x.Double, x.Bool, x.String, false, nil, false, nil, false, nil, false, nil, true)
}

func unmarshalUnion(data []byte, pi **int64, pf **float64, pb **bool, ps **string, haveArray bool, pa interface{}, haveObject bool, pc interface{}, haveMap bool, pm interface{}, haveEnum bool, pe interface{}, nullable bool) (bool, error) {
	if pi != nil {
			*pi = nil
	}
	if pf != nil {
			*pf = nil
	}
	if pb != nil {
			*pb = nil
	}
	if ps != nil {
			*ps = nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
			return false, err
	}

	switch v := tok.(type) {
	case json.Number:
			if pi != nil {
					i, err := v.Int64()
					if err == nil {
							*pi = &i
							return false, nil
					}
			}
			if pf != nil {
					f, err := v.Float64()
					if err == nil {
							*pf = &f
							return false, nil
					}
					return false, errors.New("Unparsable number")
			}
			return false, errors.New("Union does not contain number")
	case float64:
			return false, errors.New("Decoder should not return float64")
	case bool:
			if pb != nil {
					*pb = &v
					return false, nil
			}
			return false, errors.New("Union does not contain bool")
	case string:
			if haveEnum {
					return false, json.Unmarshal(data, pe)
			}
			if ps != nil {
					*ps = &v
					return false, nil
			}
			return false, errors.New("Union does not contain string")
	case nil:
			if nullable {
					return false, nil
			}
			return false, errors.New("Union does not contain null")
	case json.Delim:
			if v == '{' {
					if haveObject {
							return true, json.Unmarshal(data, pc)
					}
					if haveMap {
							return false, json.Unmarshal(data, pm)
					}
					return false, errors.New("Union does not contain object")
			}
			if v == '[' {
					if haveArray {
							return false, json.Unmarshal(data, pa)
					}
					return false, errors.New("Union does not contain array")
			}
			return false, errors.New("Cannot handle delimiter")
	}
	return false, errors.New("Cannot unmarshal union")

}

func marshalUnion(pi *int64, pf *float64, pb *bool, ps *string, haveArray bool, pa interface{}, haveObject bool, pc interface{}, haveMap bool, pm interface{}, haveEnum bool, pe interface{}, nullable bool) ([]byte, error) {
	if pi != nil {
			return json.Marshal(*pi)
	}
	if pf != nil {
			return json.Marshal(*pf)
	}
	if pb != nil {
			return json.Marshal(*pb)
	}
	if ps != nil {
			return json.Marshal(*ps)
	}
	if haveArray {
			return json.Marshal(pa)
	}
	if haveObject {
			return json.Marshal(pc)
	}
	if haveMap {
			return json.Marshal(pm)
	}
	if haveEnum {
			return json.Marshal(pe)
	}
	if nullable {
			return json.Marshal(nil)
	}
	return nil, errors.New("Union must not be null")
}
