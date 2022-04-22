//go:build kafka
// +build kafka

package internal

import (
	"fmt"
)

type kafkaMessageStreamQueue struct {
	queue []Activity
}

const QueueSize = 10

type Activity struct {
	TimestampMs uint64 `json:"timestamp_ms"`
	Activity    bool   `json:"activity"`
}

func NewKafkaMessageStreamQueue(messages []Activity) kafkaMessageStreamQueue {
	kmsq := kafkaMessageStreamQueue{}
	for _, message := range messages {
		kmsq.queue = append(kmsq.queue, message)
		if len(kmsq.queue) > QueueSize {
			var elem Activity
			elem, kmsq.queue = kmsq.queue[0], kmsq.queue[1:]
			fmt.Printf("Dequeued: %v\n", elem)
			fmt.Printf()
		}
	}

	return kmsq
}

func (k kafkaMessageStreamQueue) Enqueue(message Activity) {
	fmt.Printf("Lenght: %d\n", len(k.queue))
	k.queue = append(k.queue, message)
	if len(k.queue) > QueueSize {
		var elem Activity
		elem, k.queue = k.queue[0], k.queue[1:]
		fmt.Printf("Dequeued: %v\n", elem)
	}
	fmt.Printf("Lenght: %d\n", len(k.queue))
}

func (k kafkaMessageStreamQueue) GetLatestByTimestamp() {
	highest := Activity{
		TimestampMs: 0,
		Activity:    false,
	}

	for i := 0; i < len(k.queue); i++ {
		item := k.queue[i]
		if highest.TimestampMs < item.TimestampMs {
			highest = item
		}
		fmt.Printf("%d: %v\n", i, item)
	}
	fmt.Printf("Highest: %v\n", highest)
}
