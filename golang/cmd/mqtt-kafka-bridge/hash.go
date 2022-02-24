package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"strconv"
)

func CheckIfNewMessageOrStore(message []byte) (new bool) {
	hashString := xxh3.Hash(message)
	zap.S().Debugf("Hash: %d", hashString)
	_, old := internal.GetMemcached(strconv.FormatUint(hashString, 10))
	new = !old
	zap.S().Debugf("New: %s", new)
	if new {
		go internal.SetMemcached(strconv.FormatUint(hashString, 10), "")
	}
	return
}
