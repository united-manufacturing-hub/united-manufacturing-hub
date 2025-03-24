// Copyright (c) 2012-2016 Eli Janssen
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package tai64

//go:generate go run ./tools/generate.go -pkg $GOPACKAGE -output offsets.go

import (
	"fmt"
	"strconv"
	"time"
)

// dt at Unix epoch:  1970-01-01 00:00:00
// Corresponding TAI: 1970-01-01 00:00:10
// TAI at Unix Epoch
const epochTai64 = (2 << 61) + 10

// GetOffsetUnix returns the TAI64 offset for a UTC unix timestamp
// returns int64 offset
func GetOffsetUnix(utime int64) int64 {
	// default offset is 10
	offset := int64(10)

	for i := tia64nSize - 1; i >= 0; i-- {
		if utime < tia64nDifferences[i].utime {
			continue
		} else {
			offset = tia64nDifferences[i].offset
			break
		}
	}
	return offset
}

// GetOffsetTime returns the TAI64 offset for a time.Time in UTC
// returns int64 offset
func GetOffsetTime(t time.Time) int64 {
	return GetOffsetUnix(t.UTC().Unix())
}

func getInvOffsetTai64(ttime int64) int64 {
	// default offset is 10
	offset := int64(10)

	// walk backwards because we are looking for
	// the "bucket" where we fit
	for i := tia64nSize - 1; i >= 0; i-- {
		t := tia64nDifferences[i]
		if ttime < (t.ttime) {
			continue
		} else {
			offset = t.offset
			break
		}
	}
	return offset
}

// FormatNano formats a time.Time as a TAI64N timestamp
// returns a string TAI64N timestamps
func FormatNano(t time.Time) string {
	t = t.UTC()
	u := t.Unix()

	if u < 0 {
		return fmt.Sprintf("@%016x%08x", epochTai64+u, t.Nanosecond())
	}
	return fmt.Sprintf("@4%015x%08x", u+GetOffsetUnix(u), t.Nanosecond())
}

// Format formats a time.Time as a TAI64 timestamp
// returns a string TAI64 timestamps
func Format(t time.Time) string {
	u := t.UTC().Unix()

	if u < 0 {
		return fmt.Sprintf("@%016x", epochTai64+u)
	}
	return fmt.Sprintf("@4%015x", u+GetOffsetUnix(u))
}

// Parse parses a TAI64 or TAI64N timestamp
// returns a time.Time and an error.
func Parse(s string) (time.Time, error) {
	var tseconds, nanoseconds int64
	if s[0] == '@' {
		s = s[1:]
	}

	if len(s) < 16 {
		return time.Time{}, fmt.Errorf("invalid tai64 time string")
	}

	i, err := strconv.ParseInt(s[0:16], 16, 64)
	if err != nil {
		return time.Time{}, err
	}
	tseconds = i
	s = s[16:]

	// Check for TAI64N or TAI64NA format
	slen := len(s)
	if slen == 8 || slen == 16 {
		// time.Time is not attoseconds granular, so
		// just pull off the TAI64N section.
		i, err := strconv.ParseInt(s[:8], 16, 64)
		if err != nil {
			return time.Time{}, err
		}
		nanoseconds = i
	}

	// if tia seconds are at least TAI epoch, we are in positive unix seconds
	if tseconds >= epochTai64 {
		// To get back to unix seconds, we need to...
		// 1. Figure out the TAI-UTC offset
		// 2. Subtract TAI epoch plus any additional offset

		// get inverse offset by Tai lookup
		offset := getInvOffsetTai64(tseconds)

		// epochTai64 already has the +10 default offset included, so adjust
		// for that when subtracting offset.
		unix := tseconds - epochTai64 - (offset - 10)

		t := time.Unix(unix, nanoseconds).UTC()
		return t, nil
	}

	// for values below TAI epoch, we can just subtract seconds from TIA epoch,
	// then change sign.
	unix := -(epochTai64 - tseconds)
	t := time.Unix(unix, nanoseconds).UTC()
	return t, nil
}
