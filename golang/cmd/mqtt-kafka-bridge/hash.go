package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"strconv"
)

func CheckIfNewMessageOrStore(message []byte, topic string) (new bool) {
	hashStringMessage := xxh3.Hash(message)
	hashStringTopic := xxh3.Hash([]byte(topic))
	hashString := strconv.FormatUint(hashStringMessage, 10) + strconv.FormatUint(hashStringTopic, 10)
	zap.S().Debugf("Hash: %d", hashString)
	_, old := internal.GetMemcached(hashString)
	new = !old
	zap.S().Debugf("New: %s", new)
	if new {
		go internal.SetMemcached(hashString, "")
	}
	return
}
