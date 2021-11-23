package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"strconv"
)

func CheckIfNewMessageOrStore(message []byte) (new bool) {
	hashString := xxh3.Hash(message)
	zap.S().Infof("Hash: %d", hashString)
	old, _ := internal.GetTiered(strconv.FormatUint(hashString, 10))
	new = !old
	zap.S().Infof("New: %s", new)
	if new {
		go internal.SetTieredShortTerm(strconv.FormatUint(hashString, 10), "")
	}
	return
}
