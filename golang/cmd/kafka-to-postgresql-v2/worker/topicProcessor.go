package worker

import (
	"errors"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"regexp"
	"strings"
)

// Test this regex at https://regex101.com/r/VQAUvY/1
var topicRegex = regexp.MustCompile(`^umh\.v1\.(?P<enterprise>[\w-_]+)\.((?P<site>[\w-_]+)\.)?((?P<area>[\w-_]+)\.)?((?P<productionLine>[\w-_]+)\.)?((?P<workCell>[\w-_]+)\.)?((?P<originId>[\w-_]+)\.)?_(?P<usecase>(historian)|(analytics))(\.(?P<tag>[\w-_]+))?$`)

func recreateTopic(msg *shared.KafkaMessage) (*sharedStructs.TopicDetails, error) {
	topic := strings.Builder{}
	topic.WriteString(msg.Topic)
	if len(msg.Key) > 0 {
		key := string(msg.Key)
		topicHasDot := strings.HasSuffix(msg.Topic, ".")
		keyHasDot := strings.HasPrefix(key, ".")
		if topicHasDot && keyHasDot {
			// Topic ends with dot and string has dot prefix
			topic.WriteString(key[1:])
		} else if (topicHasDot && !keyHasDot) || (!topicHasDot && keyHasDot) {
			// Topic ends with dot and string has no dot
			topic.WriteString(key)
		} else if !topicHasDot && !keyHasDot {
			topic.WriteRune('.')
			topic.WriteString(key)
		}
	}

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

	return &sharedStructs.TopicDetails{
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
