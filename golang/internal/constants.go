package internal

import "time"

var OneSecond = 1 * time.Second
var FiveSeconds = 5 * time.Second
var TenSeconds = 10 * time.Second
var FifteenSeconds = 15 * time.Second

var FirstJan1970 = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
var UnixEpoch = FirstJan1970
