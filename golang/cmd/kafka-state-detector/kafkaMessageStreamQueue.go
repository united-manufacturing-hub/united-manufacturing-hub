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

import "github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"

type kafkaMessageStreamQueue struct {
	queue []datamodel.Activity
}

const QueueSize = 100

func NewKafkaMessageStreamQueue(messages []datamodel.Activity) kafkaMessageStreamQueue {
	kmsq := kafkaMessageStreamQueue{}
	for _, message := range messages {
		kmsq.queue = append(kmsq.queue, message)
		for len(kmsq.queue) > QueueSize {
			kmsq.queue = kmsq.queue[1:]
		}
	}

	return kmsq
}

func (k *kafkaMessageStreamQueue) Enqueue(message datamodel.Activity) {
	k.queue = append(k.queue, message)
	for len(k.queue) > QueueSize {
		k.queue = k.queue[1:]
	}
}

func (k kafkaMessageStreamQueue) GetLatestByTimestamp() datamodel.Activity {
	highest := datamodel.Activity{
		TimestampMs: 0,
		Activity:    false,
	}

	for i := 0; i < len(k.queue); i++ {
		item := k.queue[i]
		if highest.TimestampMs < item.TimestampMs {
			highest = item
		}
	}
	return highest
}
