package push

import (
	"context"
	"fmt"
	http2 "net/http"
	"sync/atomic"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/safejson"

	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/error_handler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/fail"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/tracing"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/watchdog"
	"go.uber.org/zap"
)

type DeadLetter struct {
	messages      []models.UMHMessage
	cookies       map[string]string
	retryAttempts int
}

func DefaultDeadLetterChanBuffer() chan DeadLetter {
	return make(chan DeadLetter, 1000)
}

func DefaultBackoffPolicy() *tools.Backoff {
	// 5 seconds is half the time until the frontend will consider the backend as dead
	return tools.NewBackoff(1*time.Millisecond, 2*time.Millisecond, 5*time.Second, tools.BackoffPolicyExponential)
}

type Pusher struct {
	instanceUUID           uuid.UUID
	outboundMessageChannel chan *models.UMHMessage
	deadletterCh           chan DeadLetter
	dog                    watchdog.Iface
	jwt                    atomic.Value
	watcherUUID            uuid.UUID
	backoff                *tools.Backoff
	insecureTLS            bool
}

func NewPusher(instanceUUID uuid.UUID, jwt string, dog watchdog.Iface, outboundChannel chan *models.UMHMessage, deadletterCh chan DeadLetter, backoff *tools.Backoff, insecureTLS bool) *Pusher {
	p := Pusher{
		instanceUUID:           instanceUUID,
		outboundMessageChannel: outboundChannel,
		deadletterCh:           deadletterCh,
		jwt:                    atomic.Value{},
		dog:                    dog,
		backoff:                backoff,
		insecureTLS:            insecureTLS,
	}
	p.jwt.Store(jwt)
	return &p
}

func (p *Pusher) UpdateJWT(jwt string) {
	p.jwt.Store(jwt)
}
func (p *Pusher) Start() {
	go p.push()
}

func (p *Pusher) Push(message models.UMHMessage) {
	if len(p.outboundMessageChannel) == cap(p.outboundMessageChannel) {
		zap.S().Warnf("Outbound message channel is full !")
		if p.watcherUUID != uuid.Nil {
			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
		}
	}
	p.outboundMessageChannel <- &models.UMHMessage{
		InstanceUUID: p.instanceUUID,
		Content:      message.Content,
		Email:        message.Email,
	}
	// zap.S().Debugf("Pushed message: %d", len(p.outboundMessageChannel))
}

func (p *Pusher) push() {
	boPostRequest := p.backoff
	p.watcherUUID = p.dog.RegisterHeartbeat("push", 10, 600, false)
	var ticker = time.NewTicker(10 * time.Millisecond)
	for {
		select {
		case <-ticker.C:

			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_OK)

			messages := p.outBoundMessages()
			if len(messages) == 0 {
				continue
			}

			// Start a new transaction for this push cycle
			// We only do this if we have messages to push
			transaction := sentry.StartTransaction(context.Background(), "push.cycle")

			// Create a span for message batch processing
			var span *sentry.Span
			if transaction != nil {
				span = transaction.StartChild("process.message_batch")
				// Add trace IDs from all messages in batch
				for _, msg := range messages {
					if msg.Metadata != nil && span != nil {
						tracing.AddTraceUUIDToSpan(span, msg.Metadata.TraceID)
					}
				}
			}

			var cookies = map[string]string{
				"token": p.jwt.Load().(string),
			}

			payload := backend_api_structs.PushPayload{
				UMHMessages: messages,
			}
			_, err, status := http.PostRequest[any, backend_api_structs.PushPayload](transaction.Context(), http.PushEndpoint, &payload, nil, &cookies, p.insecureTLS)
			if err != nil {
				// Start a new span for the error handling
				var errorSpan *sentry.Span
				if transaction != nil {
					errorSpan = transaction.StartChild("error.handling")
					errorSpan.Status = sentry.SpanStatusInternalError
				}
				error_handler.ReportHTTPErrors(err, status, string(http.PushEndpoint), "POST", &payload, nil)
				p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
				if status == http2.StatusBadRequest {
					// Its bit fuzzy here to determine the error code since the PostRequest does not return the error code.
					// Todo: Need to refactor the PostRequest to return the error code.
					// If the error is 400, drop the message, then the message is invalid.
					// Hence do not reenqueue the message to the deadletter channel.
					boPostRequest.IncrementAndSleep()
					if errorSpan != nil {
						errorSpan.Finish()
					}
					continue
				}
				// In case of an error, push the message back to the deadletter channel.
				go enqueueToDeadLetterChannel(p.deadletterCh, messages, cookies, 0)
				boPostRequest.IncrementAndSleep()
				if errorSpan != nil {
					errorSpan.Finish()
				}
				if span != nil {
					span.Finish()
				}
				if transaction != nil {
					transaction.Finish()
				}
				continue
			}
			error_handler.ResetErrorCounter()
			boPostRequest.Reset()
			span.Finish()
			transaction.Finish()
		case d, ok := <-p.deadletterCh:
			if !ok {
				continue
			}

			if len(d.messages) == 0 {
				continue
			}

			p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_OK)
			// Retry the messages in deadletter channel only thrice. If it fails after 3 retryAttempts, log the message and drop.
			if d.retryAttempts > 2 {
				messageJson, err := safejson.Marshal(d.messages)
				if err == nil {
					fail.ErrorBatchedfWithAttachment("Dropping the message after 3 retry attempts", []sentry.Attachment{
						{
							Filename:    "dropped.json",
							ContentType: "application/json",
							Payload:     messageJson,
						},
					})
				} else {
					fail.ErrorBatchedfWithAttachment("Dropping the message after 3 retry attempts", []sentry.Attachment{
						{
							Filename:    "dropped.error.txt",
							ContentType: "text/plain",
							Payload:     []byte(fmt.Sprintf("Failed to marshal message to json: %s", err)),
						},
					})
				}
				continue
			}
			d.retryAttempts++
			_, err, _ := http.PostRequest[any, backend_api_structs.PushPayload](nil, http.PushEndpoint, &backend_api_structs.PushPayload{UMHMessages: d.messages}, nil, &d.cookies, p.insecureTLS)
			if err != nil {
				p.dog.ReportHeartbeatStatus(p.watcherUUID, watchdog.HEARTBEAT_STATUS_WARNING)
				boPostRequest.IncrementAndSleep()
				// In case of an error, push the message back to the deadletter channel.
				go enqueueToDeadLetterChannel(p.deadletterCh, d.messages, d.cookies, d.retryAttempts)
			}
			boPostRequest.Reset()
		}
	}
}

func enqueueToDeadLetterChannel(deadLetterCh chan DeadLetter, messages []models.UMHMessage, cookies map[string]string, retryAttempt int) {
	zap.S().Debugf("Enqueueing to deadletter channel to push messages: %v with retry attempts: %d", messages, retryAttempt)

	// Convert msg to json for attachment
	messageJson, err := safejson.Marshal(messages)

	if err == nil {
		fail.WarnBatchedfWithAttachment("Enqueueing to deadletter channel to push messages: Attempt %d", []sentry.Attachment{
			{
				Filename:    "deadletter.log",
				ContentType: "application/json",
				Payload:     messageJson,
			},
		}, retryAttempt)
	} else {
		fail.WarnBatchedfWithAttachment("Enqueueing to deadletter channel to push messages: Attempt %d", []sentry.Attachment{
			{
				Filename:    "deadletter.error.txt",
				ContentType: "text/plain",
				Payload:     []byte(fmt.Sprintf("Failed to marshal message to json: %s", err)),
			},
		}, retryAttempt)
	}

	select {
	case _, ok := <-deadLetterCh:
		if !ok {
			// Channel is closed
			fail.ErrorBatchedf("Deadletter channel is closed, cannot enqueue messages!")
			return
		}
	case deadLetterCh <- DeadLetter{
		messages:      messages,
		cookies:       cookies,
		retryAttempts: retryAttempt,
	}:
		// Message successfully enqueued to deadletter channel. Do nothing.
	default:
		fail.ErrorBatchedf("Deadletter channel is not open or ready to receive the re-enqueued messages from the Pusher!")
	}
}

func (p *Pusher) outBoundMessages() []models.UMHMessage {
	messages := make([]models.UMHMessage, 0, len(p.outboundMessageChannel))
	if len(p.outboundMessageChannel) == 0 {
		return messages
	}
	for len(p.outboundMessageChannel) > 0 {
		msgX := <-p.outboundMessageChannel
		if msgX == nil {
			continue
		}
		messages = append(messages, *msgX)
	}
	return messages
}
