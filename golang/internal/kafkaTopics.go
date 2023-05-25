// Copyright 2023 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"errors"
	"fmt"
	"github.com/hashicorp/golang-lru"
	"regexp"
	"strings"
)

// KafkaUMHTopicRegex is the regex used to validate kafka topics
// Deprecated: Use umhV1SyntaxRegex instead
var KafkaUMHTopicRegex = `^ia\.((raw\.(\d|-|\w|_|\.)+)|(rawImage\.(\d|-|\w|_)+\.(\d|-|\w|_)+)|(((\d|-|\w|_)+)\.((\d|-|\w|_)+)\.((\d|-|\w|_)+)\.((count|scrapCount|barcode|activity|detectedAnomaly|addShift|addOrder|addProduct|startOrder|endOrder|productImage|productTag|productTagString|addParentToChild|state|cycleTimeTrigger|uniqueProduct|scrapUniqueProduct|recommendations)|(processValue(\.(\d|-|\w|_)+)*)|(processValueString(\.(\d|-|\w|_)+)*))))$`

// KafkaUMHTopicCompiledRegex is the compiled regex used to validate kafka topics
// Deprecated: Use umhV1SyntaxCompiledRegex instead
var KafkaUMHTopicCompiledRegex = regexp.MustCompile(KafkaUMHTopicRegex)

// IsKafkaTopicValid returns true if the given topic is valid
// Deprecated: Use IsKafkaTopicV1Valid instead
func IsKafkaTopicValid(topic string) bool {
	return KafkaUMHTopicCompiledRegex.MatchString(topic)
}

// TopicInformation contains information about a kafka topic
// Deprecated: Use TopicInformationV1 instead
type TopicInformation struct {
	TransmitterId      *string
	MacAddressOfCamera *string
	AssetId            string
	Location           string
	CustomerId         string
	Topic              string
	ExtendedTopics     []string
}

var regexLruCache, _ = lru.New(100) //nolint:errcheck

// GetTopicInformationCached returns the topic information for the given topic.
// The topic information is cached in an LRU cache, resulting in up to 10x performance improvement.
// Deprecated: Use GetTopicInformationV1Cached instead
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
// Deprecated: Use getTopicInformationV1 instead
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

const (
	// umhV1SyntaxRegex is the regex used to validate kafka topics
	// It only checks if the syntax is correct.
	// Please use GetTopicInformationV1Cached to get a deep check of the topic
	umhV1SyntaxRegex   = `^umh\.v1\.(?P<enterprise>[\w\-_]+)\.(?P<site>[\w\-_]+)\.(?P<area>[\w\-_]+)\.(?P<productionLine>[\w\-_]+)\.(?P<workCell>[\w\-_]+)\.(?P<tagGroup>raw|standard|custom)\.([\w\-_.]+\.?)$`
	umhV1CustomRegex   = `^umh\.v1\.(?P<enterprise>[\w\-_]+)\.(?P<site>[\w\-_]+)\.(?P<area>[\w\-_]+)\.(?P<productionLine>[\w\-_]+)\.(?P<workCell>[\w\-_]+)\.(?P<tagGroup>custom)\.(?P<tag>processValueString|processValue)(\.(?P<label>[\w\-_.]+))?$`
	umhV1RawRegex      = `^umh\.v1\.(?P<enterprise>[\w\-_]+)\.(?P<site>[\w\-_]+)\.(?P<area>[\w\-_]+)\.(?P<productionLine>[\w\-_]+)\.(?P<workCell>[\w\-_]+)\.(?P<tagGroup>raw)\.((rawImage\.(?P<transmitterId>[\w\-_]+)\.)(?P<macAddress>([0-9A-Fa-f]{2}-){5}([0-9A-Fa-f]{2})|)|(raw))`
	umhV1StandardRegex = `^umh\.v1\.(?P<enterprise>[\w\-_]+)\.(?P<site>[\w\-_]+)\.(?P<area>[\w\-_]+)\.(?P<productionLine>[\w\-_]+)\.(?P<workCell>[\w\-_]+)\.(?P<tagGroup>standard)\.(((?P<job>job)\.(?P<jobMethod>add|delete|end))|((?P<shift>shift)\.(?P<shiftMethod>add|delete))|((?P<producttype>product-type)\.(?P<producttypeMethod>add))|((?P<product>product)\.(?P<productMethod>add|overwrite))|((?P<state>state)\.(?P<stateMethod>add|overwrite|activity|reason)))`
)

var umhV1SyntaxCompiledRegex = regexp.MustCompile(umhV1SyntaxRegex)
var umhV1CustomCompiledRegex = regexp.MustCompile(umhV1CustomRegex)
var umhV1RawCompiledRegex = regexp.MustCompile(umhV1RawRegex)
var umhV1StandardCompiledRegex = regexp.MustCompile(umhV1StandardRegex)

// IsKafkaTopicV1Valid checks if the given topic is syntactically valid
func IsKafkaTopicV1Valid(topic string) bool {
	return umhV1SyntaxCompiledRegex.MatchString(topic)
}

type TopicInformationV1 struct {
	Label          *string
	Method         *string
	TransmitterId  *string
	MacAddress     *string
	Enterprise     string
	Site           string
	Area           string
	ProductionLine string
	WorkCell       string
	TagGroup       string
	Tag            string
}

func (t *TopicInformationV1) Display() string {
	str := fmt.Sprintf(
		"umh.v1.%s.%s.%s.%s.%s.%s",
		t.Enterprise,
		t.Site,
		t.Area,
		t.ProductionLine,
		t.WorkCell,
		t.TagGroup)
	if t.Tag != "" {
		str += fmt.Sprintf(".%s", t.Tag)
	}

	if t.Label != nil {
		str += fmt.Sprintf(".%s", *t.Label)
	}
	if t.Method != nil {
		str += fmt.Sprintf(".%s", *t.Method)
	}
	if t.TransmitterId != nil {
		str += fmt.Sprintf(".%s", *t.TransmitterId)
	}
	if t.MacAddress != nil {
		str += fmt.Sprintf(".%s", *t.MacAddress)
	}
	return str
}

func GetTopicInformationV1Cached(topic string) (*TopicInformationV1, error) {
	value, ok := regexLruCache.Get(topic)
	if ok {
		return value.(*TopicInformationV1), nil
	}
	gti, err := getTopicInformationV1(topic)
	if err != nil {
		return nil, err
	}
	regexLruCache.Add(topic, gti)
	return gti, nil
}

func getTopicInformationV1(topic string) (*TopicInformationV1, error) {
	if !IsKafkaTopicV1Valid(topic) {
		return nil, errors.New("invalid topic")
	}

	result := regexGetMap(topic, umhV1SyntaxCompiledRegex)

	var topicInformation TopicInformationV1
	topicInformation.Enterprise = result["enterprise"]
	topicInformation.Site = result["site"]
	topicInformation.Area = result["area"]
	topicInformation.ProductionLine = result["productionLine"]
	topicInformation.WorkCell = result["workCell"]
	topicInformation.TagGroup = result["tagGroup"]

	if len(topicInformation.Enterprise) == 0 {
		return nil, errors.New("invalid topic")
	}
	if len(topicInformation.Site) == 0 {
		return nil, errors.New("invalid topic")
	}
	if len(topicInformation.Area) == 0 {
		return nil, errors.New("invalid topic")
	}
	if len(topicInformation.ProductionLine) == 0 {
		return nil, errors.New("invalid topic")
	}
	if len(topicInformation.WorkCell) == 0 {
		return nil, errors.New("invalid topic")
	}
	if len(topicInformation.TagGroup) == 0 {
		return nil, errors.New("invalid topic")
	}

	var err error
	switch topicInformation.TagGroup {
	case "custom":
		getTopicInformationV1Custom(topic, &topicInformation)
	case "raw":
		getTopicInformationV1Raw(topic, &topicInformation)
	case "standard":
		err = getTopicInformationV1Standard(topic, &topicInformation)
	default:
		return nil, errors.New("invalid topic")
	}

	return &topicInformation, err
}

func getTopicInformationV1Standard(topic string, t *TopicInformationV1) error {
	result := regexGetMap(topic, umhV1StandardCompiledRegex)

	if result["job"] == "job" {
		t.Tag = "job"
		method := result["jobMethod"]
		t.Method = &method
	} else if result["shift"] == "shift" {
		t.Tag = "shift"
		method := result["shiftMethod"]
		t.Method = &method
	} else if result["producttype"] == "product-type" {
		t.Tag = "product-type"
		method := result["producttypeMethod"]
		t.Method = &method
	} else if result["product"] == "product" {
		t.Tag = "product"
		method := result["productMethod"]
		t.Method = &method
	} else if result["state"] == "state" {
		t.Tag = "state"
		method := result["stateMethod"]
		t.Method = &method
	} else {
		return errors.New("invalid topic")
	}
	return nil
}

func getTopicInformationV1Raw(topic string, t *TopicInformationV1) {
	result := regexGetMap(topic, umhV1RawCompiledRegex)

	if result["macAddress"] != "" {
		t.Tag = "rawImage"
		var mac = result["macAddress"]
		var transmitterId = result["transmitterId"]
		t.MacAddress = &mac
		t.TransmitterId = &transmitterId
	} else {
		t.Tag = "raw"
	}
}

func getTopicInformationV1Custom(topic string, topicInformation *TopicInformationV1) {
	result := regexGetMap(topic, umhV1CustomCompiledRegex)

	topicInformation.Tag = result["tag"]
	if result["label"] != "" {
		var label = result["label"]
		topicInformation.Label = &label
	}

}

func regexGetMap(topic string, regex *regexp.Regexp) map[string]string {
	match := regex.FindStringSubmatch(topic)
	result := make(map[string]string)
	subexpressions := regex.SubexpNames()
	if len(match) < len(subexpressions) {
		fmt.Printf("Match %v is shorter than subexpressions %v for topic: %s", match, subexpressions, topic)
	}
	for i, name := range subexpressions {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}

	return result
}
