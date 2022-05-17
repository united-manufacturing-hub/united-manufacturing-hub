package main

import (
	"github.com/united-manufacturing-hub/umh-lib/v2/other"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"strconv"
)

func CheckIfNewMessageOrStore(message []byte) (new bool) {
	hashString := xxh3.Hash(message)
	zap.S().Debugf("Hash: %d", hashString)
	_, old := other.GetMemcached(strconv.FormatUint(hashString, 10))
	new = !old
	zap.S().Debugf("New: %s", new)
	if new {
		go other.SetMemcached(strconv.FormatUint(hashString, 10), "")
	}
	return
}
