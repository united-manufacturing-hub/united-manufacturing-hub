package main

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"github.com/zeebo/xxh3"
	"strconv"
)

func CheckIfNewMessageOrStore(message []byte) (new bool) {
	hashString := xxh3.Hash(message)
	new, _ = internal.GetTiered(strconv.FormatUint(hashString, 10))
	if new {
		go internal.SetTieredShortTerm(strconv.FormatUint(hashString, 10), "")
	}
	return
}
