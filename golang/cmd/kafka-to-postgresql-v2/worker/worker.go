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
					zap.S().Warnf("Failed to insert work-order.create %+v: %s", parsed, err)
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
				err = p.UpdateWorkOrderSetStart(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert work-order.start %+v: %s", parsed, err)
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
				err = p.UpdateWorkOrderSetStop(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert work-order.stop %+v: %s", parsed, err)
					k.MarkMessage(msg)
					continue
				}
			case "product.add":
				parsed, err := parseProductAdd(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse product.add %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.InsertProductAdd(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert product.add %+v: %s", parsed, err)
					k.MarkMessage(msg)
					continue
				}
			case "product.setBadQuantity":
				parsed, err := parseProductSetBadQuantity(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse product.setBadQuantity %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.UpdateBadQuantityForProduct(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert product.setBadQuantity %+v: %s", parsed, err)
					k.MarkMessage(msg)
					continue
				}
			case "product-type.create":
				parsed, err := parseProductTypeCreate(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse product-type.create %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.InsertProductTypeCreate(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert product-type.create %+v: %s", parsed, err)
					k.MarkMessage(msg)
					continue
				}
			case "shift.add":
				parsed, err := parseShiftAdd(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse shift.add %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.InsertShiftAdd(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert shift.add %+v: %s", parsed, err)
					k.MarkMessage(msg)
					continue
				}
			case "shift.delete":
				parsed, err := parseShiftDelete(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse shift.delete %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.DeleteShiftByStartTime(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert shift.delete %+v: %s", parsed, err)
					k.MarkMessage(msg)
					continue
				}
			case "state.add":
				parsed, err := parseStateAdd(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse state.add %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.InsertStateAdd(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert state.add %+v: %s", parsed, err)
					k.MarkMessage(msg)
					continue
				}
			case "state.overwrite":
				parsed, err := parseStateOverwrite(msg.Value)
				if err != nil {
					zap.S().Warnf("Failed to parse state.overwrite %+v: %s", msg, err)
					k.MarkMessage(msg)
					continue
				}
				err = p.OverwriteStateByStartEndTime(parsed, topic)
				if err != nil {
					zap.S().Warnf("Failed to insert state.overwrite %+v: %s", parsed, err)
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
