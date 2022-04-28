package internal

import "regexp"

var KafkaUMHTopicRegex = `^ia\.((raw\.(\d|-|\w|_)+\.(\d|-|\w|_)+\.(\d|-|\w|_)+(\.(\d|-|\w|_)+)?)|(rawImage\.(\d|-|\w|_)+\.(\d|-|\w|_)+)|((\d|-|\w|_)+\.(\d|-|\w|_)+\.(\d|-|\w|_)+\.((count|scrapCount|barcode|activity|detectedAnomaly|addShift|addOrder|addProduct|startOrder|endOrder|productImage|productTag|productTagString|addParentToChild|state|cycleTimeTrigger|uniqueProduct|scrapUniqueProduct|recommendations)|(processValue(\.(\d|-|\w|_)+)*))))$`
var KafkaUMHTopicCompiledRegex = regexp.MustCompile(KafkaUMHTopicRegex)

func IsKafkaTopicValid(topic string) bool {
	return KafkaUMHTopicCompiledRegex.MatchString(topic)
}
