package worker

import (
	"sync"
	"time"

	"github.com/united-manufacturing-hub/Sarama-Kafka-Wrapper-2/pkg/kafka/shared"
	"github.com/united-manufacturing-hub/umh-utils/env"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/kafka"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/kafka-to-postgresql-v2/postgresql"
	"go.uber.org/zap"
)

type Worker struct {
	kafka    *kafka.Connection
	postgres *postgresql.Connection
}

var worker *Worker
var once sync.Once

func GetOrInit() *Worker {
	once.Do(func() {
		worker = &Worker{
			kafka:    kafka.GetOrInit(),
			postgres: postgresql.GetOrInit(),
		}
		worker.startWorkLoop()
	})
	return worker
}

func (w *Worker) startWorkLoop() {
	zap.S().Debugf("Started work loop")
	workerMultiplier, err := env.GetAsInt("WORKER_MULTIPLIER", false, 16)
	if err != nil {
		zap.S().Fatalf("Failed to get WORKER_MULTIPLIER from env: %s", err)
	}
	messageChannel := w.kafka.GetMessages()
	zap.S().Debugf("Started using %d workers (logical cores * WORKER_MULTIPLIER)", workerMultiplier)
	for i := 0; i < /*runtime.NumCPU()*workerMultiplier*/ 1; i++ {
		go handleParsing(messageChannel, i)
	}
	zap.S().Debugf("Started all workers")
}

func handleParsing(msgChan <-chan *shared.KafkaMessage, i int) {
	k := kafka.GetOrInit()
	p := postgresql.GetOrInit()
	messagesHandled := 0
	now := time.Now()
	for {
		msg := <-msgChan
		topic, err := recreateTopic(msg)
		if err != nil {
			zap.S().Warnf("Failed to parse message %+v into topic: %s", msg, err)
			k.MarkMessage(msg)
			continue
		}
		if topic == nil {
			zap.S().Fatalf("topic is null, after successful parsing, this should never happen !: %+v", msg)
		}

		origin, hasOrigin := msg.Headers["x-origin"]
		if !hasOrigin {
			origin = "unknown"
		}

		switch topic.Schema {
		case "historian":
			payload, timestampMs, err := parseHistorianPayload(msg.Value, topic.Tag)
			if err != nil {
				zap.S().Warnf("Failed to parse payload %+v for message: %s ", msg, err)
				k.MarkMessage(msg)
				continue
			}
			err = p.InsertHistorianValue(payload, timestampMs, origin, topic)
			if err != nil {
				zap.S().Warnf("Failed to insert historian numerical value %+v: %s [%+v]", msg, err, payload)
				k.MarkMessage(msg)
				continue
			}
		case "analytics":
			zap.S().Warnf("Analytics not yet supported, ignoring: %+v", msg)
			switch topic.Tag {
			case "work-order.create":
				parsed, err := parseWorkOrderCreate(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse work-order.create %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.InsertWorkOrderCreate(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert work-order.create %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
			case "work-order.start":
				parsed, err := parseWorkOrderStart(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse work-order.start %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.InsertWorkOrderStart(parsed)
				if err != nil {
					zap.S().Warnf("Failed to insert work-order.start %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
			case "work-order.stop":
				parsed, err := parseWorkOrderStop(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse work-order.stop %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.InsertWorkOrderStop(parsed)
				if err != nil {
					zap.S().Warnf("Failed to insert work-order.stop %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
			}
		default:
			zap.S().Errorf("Unknown usecase %s", topic.Schema)
		}
		k.MarkMessage(msg)
		messagesHandled++
		elapsed := time.Since(now)
		if int(elapsed.Seconds())%10 == 0 {
			zap.S().Debugf("handleParsing [%d] handled %d messages in %s (%f msg/s) [%d/%d]", i, messagesHandled, elapsed, float64(messagesHandled)/elapsed.Seconds(), len(msgChan), cap(msgChan))
		}
	}
}
