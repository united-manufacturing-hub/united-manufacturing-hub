package worker

import (
	"errors"
	"regexp"
	"strings"

	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
)

// Test this regex at https://regex101.com/r/VQAUvY/4
var topicRegex = regexp.MustCompile(`^umh\.v1\.(?P<enterprise>[\w-_]+)\.((?P<site>[\w-_]+)\.)?((?P<area>[\w-_]+)\.)?((?P<productionLine>[\w-_]+)\.)?((?P<workCell>[\w-_]+)\.)?((?P<originId>[\w-_]+)\.)?_(?P<usecase>(historian)|(analytics))\.(?P<tag>(?:[\w-_.]+\w)+)$`)

func recreateTopic(msg *shared.KafkaMessage) (*sharedStructs.TopicDetails, error) {
	topic := strings.Builder{}
	// Trim leading and trailing dots from the topic
	topic.WriteString(strings.Trim(msg.Topic, "."))
	if len(msg.Key) > 0 {
		topic.WriteRune('.')
		// Trim leading and trailing dots from the key
		topic.WriteString(strings.Trim(string(msg.Key), "."))
	}

	matches := topicRegex.FindStringSubmatch(topic.String())
	if matches == nil {
		return nil, errors.New("invalid topic format")
	}

	// Directly creating and filling the TopicDetails struct
	return &sharedStructs.TopicDetails{
		Enterprise:     getMatch(matches, "enterprise"),
		Site:           getMatch(matches, "site"),
		Area:           getMatch(matches, "area"),
		ProductionLine: getMatch(matches, "productionLine"),
		WorkCell:       getMatch(matches, "workCell"),
		OriginId:       getMatch(matches, "originId"),
		Schema:         getMatch(matches, "usecase"),
		Tag:            getMatch(matches, "tag"),
	}, nil
}

// getMatch safely retrieves the match from the regular expression results
func getMatch(matches []string, name string) string {
	for i, n := range topicRegex.SubexpNames() {
		if n == name {
			return matches[i]
		}
	}
	return ""
}
