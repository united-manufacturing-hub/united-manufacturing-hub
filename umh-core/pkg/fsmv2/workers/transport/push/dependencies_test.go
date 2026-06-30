// Copyright 2025 UMH Systems GmbH
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

package push_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

type mockTransport struct{}

func (m *mockTransport) Authenticate(_ context.Context, _ types.AuthRequest) (types.AuthResponse, error) {
	return types.AuthResponse{}, nil
}

func (m *mockTransport) Pull(_ context.Context, _ string) ([]*types.UMHMessage, error) {
	return nil, nil
}

func (m *mockTransport) Push(_ context.Context, _ string, _ []*types.UMHMessage) error {
	return nil
}

func (m *mockTransport) Close() {}

func (m *mockTransport) Reset() {}

type mockChannelProvider struct {
	inbound  chan<- *types.UMHMessage
	outbound <-chan *types.UMHMessage
}

func (m *mockChannelProvider) GetChannels(_ string) (
	inbound chan<- *types.UMHMessage,
	outbound <-chan *types.UMHMessage,
) {
	return m.inbound, m.outbound
}

func (m *mockChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	return 100, 0
}

func newTestChannelProvider() *mockChannelProvider {
	inboundBi := make(chan *types.UMHMessage, 100)
	outboundBi := make(chan *types.UMHMessage, 100)

	return &mockChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
}

func createParentDeps(logger deps.FSMLogger) *transportpkg.TransportDependencies {
	mt := &mockTransport{}
	identity := deps.Identity{ID: "parent-id", WorkerType: "transport"}

	return transportpkg.NewTransportDependencies(mt, deps.NewBaseDependencies(logger, nil, identity))
}

var _ = Describe("PushDependencies", func() {
	var (
		logger     deps.FSMLogger
		parentDeps *transportpkg.TransportDependencies
		identity   deps.Identity
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		transportpkg.SetChannelProvider(newTestChannelProvider())
		parentDeps = createParentDeps(logger)
		identity = deps.Identity{ID: "push-child-id", WorkerType: "push"}
	})

	AfterEach(func() {
		transportpkg.ClearChannelProvider()
	})

	Describe("NewPushDependencies", func() {
		It("should return non-nil with valid parentDeps", func() {
			d, err := push.NewPushDependencies(parentDeps, deps.NewBaseDependencies(logger, nil, identity))
			Expect(err).NotTo(HaveOccurred())
			Expect(d).NotTo(BeNil())
		})

		It("should return error with nil parentDeps", func() {
			d, err := push.NewPushDependencies(nil, deps.NewBaseDependencies(logger, nil, identity))
			Expect(err).To(HaveOccurred())
			Expect(d).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("parentDeps must not be nil"))
		})
	})

	Describe("Per-child error tracking", func() {
		var d *push.PushDependencies

		BeforeEach(func() {
			var err error
			d, err = push.NewPushDependencies(parentDeps, deps.NewBaseDependencies(logger, nil, identity))
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("GetOutboundChan", func() {
			It("should return the same channel as the parent", func() {
				Expect(d.GetOutboundChan()).To(Equal(parentDeps.GetOutboundChan()))
			})
		})

		Describe("GetTransport", func() {
			It("should return the same transport as the parent", func() {
				Expect(d.GetTransport()).To(Equal(parentDeps.GetTransport()))
			})

			It("should reflect parent transport changes", func() {
				newMt := &mockTransport{}
				parentDeps.SetTransport(newMt)
				Expect(d.GetTransport()).To(Equal(newMt))
			})
		})

		Describe("RecordTypedError", func() {
			It("should record on both child and parent", func() {
				d.RecordTypedError(types.ErrorTypeBackendRateLimit, 30*time.Second, 0, "")
				Expect(d.GetConsecutiveErrors()).To(Equal(1))
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(1))
				Expect(parentDeps.GetLastErrorType()).To(Equal(types.ErrorTypeBackendRateLimit))
			})
		})

		Describe("RecordSuccess", func() {
			It("should zero child errors but not parent errors", func() {
				parentDeps.RecordError()
				parentDeps.RecordError()
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(2))

				d.RecordError()
				d.RecordError()
				Expect(d.GetConsecutiveErrors()).To(Equal(2))
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(4))

				d.RecordSuccess()
				Expect(d.GetConsecutiveErrors()).To(Equal(0))
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(4))
			})
		})

		Describe("RecordError", func() {
			It("should record on both child and parent", func() {
				d.RecordError()
				d.RecordError()
				Expect(d.GetConsecutiveErrors()).To(Equal(2))
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(2))
			})
		})

		Describe("GetConsecutiveErrors", func() {
			It("should read from child tracker, not parent", func() {
				Expect(d.GetConsecutiveErrors()).To(Equal(0))
				parentDeps.RecordError()
				parentDeps.RecordError()
				parentDeps.RecordError()
				Expect(d.GetConsecutiveErrors()).To(Equal(0))
			})
		})

		Describe("GetLastErrorType", func() {
			It("should read from child field", func() {
				d.RecordTypedError(types.ErrorTypeNetwork, 0, 0, "")
				Expect(d.GetLastErrorType()).To(Equal(types.ErrorTypeNetwork))
				d.RecordSuccess()
				Expect(d.GetLastErrorType()).To(Equal(types.ErrorType(0)))
			})
		})

		Describe("GetDegradedEnteredAt", func() {
			It("should read from child tracker", func() {
				Expect(d.GetDegradedEnteredAt().IsZero()).To(BeTrue())
				d.RecordError()
				Expect(d.GetDegradedEnteredAt().IsZero()).To(BeFalse())
				d.RecordSuccess()
				Expect(d.GetDegradedEnteredAt().IsZero()).To(BeTrue())
			})
		})

		Describe("GetLastErrorAt", func() {
			It("should read from child tracker", func() {
				before := time.Now()
				d.RecordTypedError(types.ErrorTypeNetwork, 0, 0, "")
				Expect(d.GetLastErrorAt()).To(BeTemporally(">=", before))
			})
		})

		Describe("GetLastRetryAfter", func() {
			It("should read from child tracker", func() {
				d.RecordTypedError(types.ErrorTypeBackendRateLimit, 30*time.Second, 0, "")
				Expect(d.GetLastRetryAfter()).To(Equal(30 * time.Second))
			})
		})
	})

	Describe("StorePendingMessages nil filtering", func() {
		var d *push.PushDependencies

		BeforeEach(func() {
			var err error
			d, err = push.NewPushDependencies(parentDeps, deps.NewBaseDependencies(logger, nil, identity))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should filter nil messages from a mixed slice", func() {
			msgs := []*types.UMHMessage{
				{Content: "a"},
				nil,
				{Content: "b"},
				nil,
				{Content: "c"},
			}
			d.StorePendingMessages(msgs)
			Expect(d.PendingMessageCount()).To(Equal(3))
		})

		It("should store nothing when all messages are nil", func() {
			msgs := []*types.UMHMessage{nil, nil, nil}
			d.StorePendingMessages(msgs)
			Expect(d.PendingMessageCount()).To(Equal(0))
		})

		It("should return only non-nil messages after mixed input via drain", func() {
			msgs := []*types.UMHMessage{
				nil,
				{Content: "x"},
				nil,
				{Content: "y"},
			}
			d.StorePendingMessages(msgs)

			drained := d.DrainPendingMessages()
			Expect(drained).To(HaveLen(2))
			Expect(drained[0].Content).To(Equal("x"))
			Expect(drained[1].Content).To(Equal("y"))
			Expect(d.PendingMessageCount()).To(Equal(0))
		})
	})

	Describe("StorePendingMessages overflow", func() {
		var d *push.PushDependencies

		BeforeEach(func() {
			var err error
			d, err = push.NewPushDependencies(parentDeps, deps.NewBaseDependencies(logger, nil, identity))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should increment CounterMessagesDropped when buffer overflows", func() {
			msgs := make([]*types.UMHMessage, 1005)
			for i := range msgs {
				msgs[i] = &types.UMHMessage{Content: fmt.Sprintf("msg-%d", i)}
			}

			d.StorePendingMessages(msgs)

			drained := d.MetricsRecorder().Drain()
			Expect(drained.Counters[string(deps.CounterMessagesDropped)]).To(Equal(int64(5)))
		})
	})

	Describe("Cross-child isolation", func() {
		var (
			pushDeps *push.PushDependencies
			pullDeps *pull.PullDependencies
		)

		BeforeEach(func() {
			pushIdentity := deps.Identity{ID: "push-id", WorkerType: "push"}
			pullIdentity := deps.Identity{ID: "pull-id", WorkerType: "pull"}

			var err error
			pushDeps, err = push.NewPushDependencies(parentDeps, deps.NewBaseDependencies(logger, nil, pushIdentity))
			Expect(err).NotTo(HaveOccurred())

			pullDeps, err = pull.NewPullDependencies(parentDeps, deps.NewBaseDependencies(logger, nil, pullIdentity))
			Expect(err).NotTo(HaveOccurred())
		})

		It("push success does not mask pull errors", func() {
			pullDeps.RecordError()
			pullDeps.RecordError()
			pullDeps.RecordError()
			Expect(pullDeps.GetConsecutiveErrors()).To(Equal(3))

			pushDeps.RecordSuccess()

			Expect(pushDeps.GetConsecutiveErrors()).To(Equal(0))
			Expect(pullDeps.GetConsecutiveErrors()).To(Equal(3))
			Expect(parentDeps.GetConsecutiveErrors()).To(Equal(3))
		})

		It("pull success does not mask push errors", func() {
			pushDeps.RecordError()
			pushDeps.RecordError()
			pushDeps.RecordError()

			pullDeps.RecordSuccess()

			Expect(pullDeps.GetConsecutiveErrors()).To(Equal(0))
			Expect(pushDeps.GetConsecutiveErrors()).To(Equal(3))
			Expect(parentDeps.GetConsecutiveErrors()).To(Equal(3))
		})
	})

	Describe("BaseDependencies", func() {
		It("should have its own logger", func() {
			d, err := push.NewPushDependencies(parentDeps, deps.NewBaseDependencies(logger, nil, identity))
			Expect(err).NotTo(HaveOccurred())
			Expect(d.GetLogger()).NotTo(BeNil())
		})
	})
})

var _ = Describe("RecordTypedError status_code and error_detail emission", func() {
	var (
		buf        *bytes.Buffer
		jsonLogger deps.FSMLogger
		d          *push.PushDependencies
	)

	BeforeEach(func() {
		buf = new(bytes.Buffer)
		jsonLogger = deps.NewJSONFSMLogger(buf, deps.LevelDebug)
		transportpkg.SetChannelProvider(newTestChannelProvider())
		parentDeps := createParentDeps(jsonLogger)
		identity := deps.Identity{ID: "push-child-id", WorkerType: "push"}
		var err error
		d, err = push.NewPushDependencies(parentDeps, deps.NewBaseDependencies(jsonLogger, nil, identity))
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		transportpkg.ClearChannelProvider()
	})

	It("emits status_code and error_detail on persistent_push_failure after escalation", func() {
		detail := "HTTP 502 (server_error): error code: 502"
		for range transportpkg.ChildFailureRateConfig.MinSamples {
			d.RecordTypedError(types.ErrorTypeServerError, 0, 502, detail)
		}

		Expect(d.GetLastStatusCode()).To(Equal(502))
		Expect(d.GetLastErrorDetail()).To(Equal(detail))

		m := parseLastJSONLine(buf)
		Expect(m["msg"]).To(Equal("persistent_push_failure"))
		Expect(m["status_code"]).To(BeEquivalentTo(502))
		Expect(m["error_detail"]).To(Equal(detail))
	})

	It("RecordSuccess clears status_code and error_detail", func() {
		d.RecordTypedError(types.ErrorTypeServerError, 0, 502, "HTTP 502 (server_error): error code: 502")
		Expect(d.GetLastStatusCode()).To(Equal(502))
		d.RecordSuccess()
		Expect(d.GetLastStatusCode()).To(Equal(0))
		Expect(d.GetLastErrorDetail()).To(Equal(""))
	})
})

func parseLastJSONLine(buf *bytes.Buffer) map[string]interface{} {
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) == 0 {
		return nil
	}

	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return nil
	}

	m := make(map[string]interface{})
	ExpectWithOffset(1, json.Unmarshal([]byte(last), &m)).To(Succeed())

	return m
}
