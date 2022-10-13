package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"strconv"
)

func CheckIfNewMessageOrStore(message []byte, topic string) (isNewMessage bool) {
	hashStringMessage := xxh3.Hash(message)
	hashStringTopic := xxh3.Hash([]byte(topic))
	hashString := strconv.FormatUint(hashStringMessage, 10) + strconv.FormatUint(hashStringTopic, 10)
	zap.S().Debugf("Hash: %s", hashString)
	_, old := internal.GetMemcached(hashString)
	isNewMessage = !old
	zap.S().Debugf("New: %v", isNewMessage)
	if isNewMessage {
		go internal.SetMemcached(hashString, "")
	}
	return
}
