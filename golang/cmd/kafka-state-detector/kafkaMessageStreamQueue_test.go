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

package main

import (
	"github.com/go-playground/assert/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"math/rand"
	"testing"
	"time"
)

func TestInsert101(t *testing.T) {

	r := NewBoolRandGen()

	initialMessages := make([]datamodel.Activity, 10)
	for i := 0; i < 10; i++ {
		initialMessages[i] = datamodel.Activity{
			TimestampMs: uint64(i),
			Activity:    r.Bool(),
		}
	}

	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(initialMessages), func(i, j int) { initialMessages[i], initialMessages[j] = initialMessages[j], initialMessages[i] })

	kmsq := NewKafkaMessageStreamQueue(initialMessages)

	kmsq.Enqueue(datamodel.Activity{
		TimestampMs: uint64(101),
		Activity:    true,
	})
	kmsq.Enqueue(datamodel.Activity{
		TimestampMs: uint64(102),
		Activity:    true,
	})
	kmsq.Enqueue(datamodel.Activity{
		TimestampMs: uint64(103),
		Activity:    true,
	})
	kmsq.Enqueue(datamodel.Activity{
		TimestampMs: uint64(105),
		Activity:    true,
	})
	kmsq.Enqueue(datamodel.Activity{
		TimestampMs: uint64(104),
		Activity:    true,
	})

	latest := kmsq.GetLatestByTimestamp()
	assert.Equal(t, latest.TimestampMs, uint64(105))

}

type boolgen struct {
	src       rand.Source
	cache     int64
	remaining int
}

func (b *boolgen) Bool() bool {
	if b.remaining == 0 {
		b.cache, b.remaining = b.src.Int63(), 63
	}

	result := b.cache&0x01 == 1
	b.cache >>= 1
	b.remaining--

	return result
}

func NewBoolRandGen() *boolgen {
	return &boolgen{src: rand.NewSource(time.Now().UnixNano())}
}
