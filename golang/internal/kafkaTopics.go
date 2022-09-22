package internal

import (
	"github.com/hashicorp/golang-lru"
	"regexp"
	"strings"
)

var KafkaUMHTopicRegex = `^ia\.((raw\.(\d|-|\w|_|\.)+)|(rawImage\.(\d|-|\w|_)+\.(\d|-|\w|_)+)|(((\d|-|\w|_)+)\.((\d|-|\w|_)+)\.((\d|-|\w|_)+)\.((count|scrapCount|barcode|activity|detectedAnomaly|addShift|addOrder|addProduct|startOrder|endOrder|productImage|productTag|productTagString|addParentToChild|state|cycleTimeTrigger|uniqueProduct|scrapUniqueProduct|recommendations)|(processValue(\.(\d|-|\w|_)+)*)|(processValueString(\.(\d|-|\w|_)+)*))))$`
var KafkaUMHTopicCompiledRegex = regexp.MustCompile(KafkaUMHTopicRegex)

func IsKafkaTopicValid(topic string) bool {
	return KafkaUMHTopicCompiledRegex.MatchString(topic)
}

type TopicInformation struct {
	AssetId            string
	Location           string
	CustomerId         string
	Topic              string
	ExtendedTopics     []string
	TransmitterId      *string
	MacAddressOfCamera *string
}

var regexLruCache, _ = lru.New(100)

// GetTopicInformationCached returns the topic information for the given topic.
// The topic information is cached in an LRU cache, resulting in up to 10x performance improvement.
func GetTopicInformationCached(topic string) *TopicInformation {
	value, ok := regexLruCache.Get(topic)
	if ok {
		return value.(*TopicInformation)
	}
	gti := getTopicInformation(topic)
	regexLruCache.Add(topic, gti)
	return gti
}

// getTopicInformation returns the topic information for the given topic
// Use the cached version if possible (GetTopicInformationCached)
func getTopicInformation(topic string) *TopicInformation {
	if !IsKafkaTopicValid(topic) {
		return nil
	}

	submatch := KafkaUMHTopicCompiledRegex.FindSubmatch([]byte(topic))
	cId := string(submatch[8])
	location := string(submatch[10])
	aId := string(submatch[12])
	var baseTopic string
	var extendedTopic []string
	var transmitterId *string
	var macAddressOfCamera *string

	if submatch[11] == nil {
		if submatch[2] == nil {
			if submatch[4] == nil {
				return nil
			}
			// rawImage
			topicSplits := strings.Split(string(submatch[4]), ".")
			baseTopic = topicSplits[0]
			tId := string(submatch[5])
			maoc := string(submatch[6])
			transmitterId = &tId
			macAddressOfCamera = &maoc
		} else {
			// raw topic
			topicSplits := strings.Split(string(submatch[2]), ".")
			baseTopic = topicSplits[0]
			extendedTopic = topicSplits[1:]
		}

	} else {
		topicSplits := strings.Split(string(submatch[14]), ".")
		baseTopic = topicSplits[0]
		extendedTopic = topicSplits[1:]
	}

	return &TopicInformation{
		AssetId:            aId,
		CustomerId:         cId,
		Location:           location,
		Topic:              baseTopic,
		ExtendedTopics:     extendedTopic,
		TransmitterId:      transmitterId,
		MacAddressOfCamera: macAddressOfCamera,
	}
}
