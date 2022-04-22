package internal

import (
	"math/rand"
	"testing"
	"time"
)

func TestInsert101(t *testing.T) {

	r := NewBoolRandGen()

	initialMessages := make([]Activity, 10)
	for i := 0; i < 10; i++ {
		initialMessages[i] = Activity{
			TimestampMs: uint64(i),
			Activity:    r.Bool(),
		}
	}

	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(initialMessages), func(i, j int) { initialMessages[i], initialMessages[j] = initialMessages[j], initialMessages[i] })

	kmsq := NewKafkaMessageStreamQueue(initialMessages)

	kmsq.Enqueue(Activity{
		TimestampMs: uint64(101),
		Activity:    true,
	})
	kmsq.Enqueue(Activity{
		TimestampMs: uint64(102),
		Activity:    true,
	})
	kmsq.Enqueue(Activity{
		TimestampMs: uint64(103),
		Activity:    true,
	})
	kmsq.Enqueue(Activity{
		TimestampMs: uint64(104),
		Activity:    true,
	})
	kmsq.Enqueue(Activity{
		TimestampMs: uint64(105),
		Activity:    true,
	})

	kmsq.GetLatestByTimestamp()

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
