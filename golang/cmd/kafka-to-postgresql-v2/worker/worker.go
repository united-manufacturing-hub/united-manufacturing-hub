package worker

import (
	"errors"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/postgresql"
	"go.uber.org/zap"
	"regexp"
	"strings"
	"sync"
)

type Worker struct {
	kafka    *kafka.Connection
	postgres *postgresql.Connection
}

var worker *Worker
var once sync.Once

func Init() *Worker {
	once.Do(func() {
		worker = &Worker{
			kafka:    kafka.Init(),
			postgres: postgresql.Init(),
		}
		go worker.startWorkLoop()
	})
	return worker
}

func (w *Worker) startWorkLoop() {
	messageChannel := w.kafka.GetMessages()
	for {
		select {
		case msg := <-messageChannel:
			topic, err := recreateTopic(msg)
			if err != nil {
				zap.S().Warn("Failed to parse message %v into topic: %s", err)
				w.kafka.MarkMessage(msg)
				continue
			}
			switch topic.Usecase {
			case "historian":
				// TODO: Implement
			case "analytics":
				zap.S().Warnf("Analytics not yet supported")
			}
			w.kafka.MarkMessage(msg)
		}
	}
}

// Test this regex at https://regex101.com/r/VQAUvY/1
var topicRegex = regexp.MustCompile(`^umh\.v1\.(?P<enterprise>[\w-_]+)\.((?P<site>[\w-_]+)\.)?((?P<area>[\w-_]+)\.)?((?P<productionLine>[\w-_]+)\.)?((?P<workCell>[\w-_]+)\.)?((?P<originId>[\w-_]+)\.)?_(?P<usecase>(historian)|(analytics))(\.(?P<tag>[\w-_]+))?$`)

type TopicDetails struct {
	Enterprise     string
	Site           *string
	Area           *string
	ProductionLine *string
	WorkCell       *string
	OriginId       *string
	Usecase        string
	Tag            *string
}

func recreateTopic(msg *shared.KafkaMessage) (*TopicDetails, error) {
	topic := strings.Builder{}
	topic.WriteString(msg.Topic)
	if len(msg.Key) > 0 {
		topic.Write(msg.Key)
	}

	println(topic.String())

	matches := topicRegex.FindStringSubmatch(topic.String())
	if matches == nil {
		return nil, errors.New("invalid topic format")
	}

	result := make(map[string]*string)
	for i, name := range topicRegex.SubexpNames() {
		if i != 0 && name != "" {
			if matches[i] != "" {
				result[name] = &matches[i]
			} else {
				result[name] = nil
			}
		}
	}

	return &TopicDetails{
		Enterprise:     *result["enterprise"],
		Site:           result["site"],
		Area:           result["area"],
		ProductionLine: result["productionLine"],
		WorkCell:       result["workCell"],
		OriginId:       result["originId"],
		Usecase:        *result["usecase"],
		Tag:            result["tag"],
	}, nil
}
