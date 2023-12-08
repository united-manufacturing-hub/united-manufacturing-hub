package worker

import (
	"errors"
	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	sharedStructs "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/shared"
	"regexp"
	"strings"
)

// Test this regex at https://regex101.com/r/VQAUvY/2
var topicRegex = regexp.MustCompile(`^umh\.v1\.(?P<enterprise>[\w-_]+)\.((?P<site>[\w-_]+)\.)?((?P<area>[\w-_]+)\.)?((?P<productionLine>[\w-_]+)\.)?((?P<workCell>[\w-_]+)\.)?((?P<originId>[\w-_]+)\.)?_(?P<usecase>(historian)|(analytics))(\.(?P<tag>[\w-_.]+))?$`)

// recreateTopicREGEX recreates the topic details from the topic string
//
// Deprecated: use recreateTopic instead
func recreateTopicREGEX(msg *shared.KafkaMessage) (*sharedStructs.TopicDetails, error) {
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

	// Directly creating and filling the TopicDetails struct
	return &sharedStructs.TopicDetails{
		Enterprise:     getMatch(matches, "enterprise"),
		Site:           getMatch(matches, "site"),
		Area:           getMatch(matches, "area"),
		ProductionLine: getMatch(matches, "productionLine"),
		WorkCell:       getMatch(matches, "workCell"),
		OriginId:       getMatch(matches, "originId"),
		Usecase:        getMatch(matches, "usecase"),
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

// recreateTopic recreates the topic details from the topic string
func recreateTopic(msg *shared.KafkaMessage) (*sharedStructs.TopicDetails, error) {
	var topicDetail sharedStructs.TopicDetails

	fullTopic := constructFullTopic(msg)
	parts := strings.Split(fullTopic, ".")

	// Validate basic topic format
	if len(parts) < 3 || parts[0] != "umh" || parts[1] != "v1" {
		return nil, errors.New("invalid topic format")
	}

	// Assign enterprise as it is required
	topicDetail.Enterprise = parts[2]

	// Find the index of usecase
	usecaseIndex := findUsecaseIndex(parts)
	if usecaseIndex == -1 {
		return nil, errors.New("usecase not found in topic")
	}

	// Assign usecase
	topicDetail.Usecase = parts[usecaseIndex][1:]

	// Assign optional fields based on the usecase index
	if usecaseIndex > 3 {
		topicDetail.Site = parts[3]
	}
	if usecaseIndex > 4 {
		topicDetail.Area = parts[4]
	}
	if usecaseIndex > 5 {
		topicDetail.ProductionLine = parts[5]
	}
	if usecaseIndex > 6 {
		topicDetail.WorkCell = parts[6]
	}
	if usecaseIndex > 7 {
		topicDetail.OriginId = parts[7]
	}
	if usecaseIndex < len(parts)-1 {
		topicDetail.Tag = strings.Join(parts[usecaseIndex+1:], ".")
	}

	return &topicDetail, nil
}

// constructFullTopic constructs the full topic string from KafkaMessage
func constructFullTopic(msg *shared.KafkaMessage) string {
	topic := msg.Topic
	if len(msg.Key) > 0 {
		key := string(msg.Key)
		if strings.HasSuffix(topic, ".") && strings.HasPrefix(key, ".") {
			// Remove the dot from the key
			topic += key[1:]
		} else if !strings.HasSuffix(topic, ".") && !strings.HasPrefix(key, ".") {
			// Add a dot between topic and key
			topic += "." + key
		} else {
			// Directly concatenate topic and key
			topic += key
		}
	}
	return topic
}

// findUsecaseIndex finds the index of the usecase segment in the topic parts
func findUsecaseIndex(parts []string) int {
	for i, part := range parts {
		if part == "_historian" || part == "_analytics" {
			return i
		}
	}
	return -1
}
