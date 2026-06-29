package gatekeeper

import (
	"context"
	"crypto/x509"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// mockCertHandler implements certificatehandler.Handler for testing.
type mockCertHandler struct {
	cert   *x509.Certificate
	rootCA *x509.Certificate
}

func (m *mockCertHandler) Certificate(_ string) *x509.Certificate              { return m.cert }
func (m *mockCertHandler) IntermediateCerts(_ string) []*x509.Certificate      { return nil }
func (m *mockCertHandler) RootCA() *x509.Certificate                           { return m.rootCA }
func (m *mockCertHandler) FetchAllCerts(_ context.Context) error               { return nil }
func (m *mockCertHandler) FetchCertForEmail(_ context.Context, _ string) error { return nil }
func (m *mockCertHandler) Subscribers() []string                               { return nil }
func (m *mockCertHandler) HasSubHandler() bool                                 { return false }
func (m *mockCertHandler) SetSubHandler(_ certificatehandler.SubHandler)       {}

// Compile-time check
var _ certificatehandler.Handler = (*mockCertHandler)(nil)

// mockValidator implements validator.Validator for testing.
type mockValidator struct {
	allowed    bool
	lastAction string
	callCount  int
}

func (m *mockValidator) ValidateUserPermissions(_ *x509.Certificate, action, _ string, _ *x509.Certificate, _ []*x509.Certificate) (bool, error) {
	m.callCount++
	m.lastAction = action
	return m.allowed, nil
}
func (m *mockValidator) DecryptRootCA(_, _, _ string) (string, error) { return "", nil }

func encodeAction(actionType string) string {
	c, err := encoding.EncodeMessageFromUserToUMHInstance(models.UMHMessageContent{
		MessageType: models.Action,
		Payload:     map[string]interface{}{"actionType": actionType},
	})
	Expect(err).NotTo(HaveOccurred())
	return c
}

var _ = Describe("Gatekeeper inbound pipeline", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		g      *Gatekeeper
		in     chan *types.UMHMessage
		cachi  *mockCertHandler
		vali   *mockValidator
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		cachi = &mockCertHandler{cert: &x509.Certificate{}, rootCA: &x509.Certificate{}}
		vali = &mockValidator{allowed: true}
		in = make(chan *types.UMHMessage, 10)
		out := make(chan *types.UMHMessage, 10)
		g = New(in, out, cachi, vali, zap.NewNop().Sugar())
		g.Start(ctx)
	})

	AfterEach(func() {
		g.Stop()
		cancel()
	})

	It("allowed action flows through full pipeline", func() {
		in <- &types.UMHMessage{Content: encodeAction("deploy-protocol-converter"), Email: "user@test.com", TraceID: "t1"}

		var msg *types.MessageWithSender
		Eventually(g.VerifiedInboundChan()).Should(Receive(&msg))

		Expect(msg.Content.MessageType).To(Equal(models.Action))
		Expect(msg.SenderEmail).To(Equal("user@test.com"))
		Expect(msg.TraceID).To(Equal("t1"))
		Expect(vali.lastAction).To(Equal("deploy-protocol-converter"))
	})

	It("drops when no cert cached", func() {
		cachi.cert = nil
		in <- &types.UMHMessage{Content: encodeAction("deploy"), Email: "user@test.com"}
		Consistently(g.VerifiedInboundChan()).ShouldNot(Receive())
		Expect(vali.callCount).To(Equal(0))
	})

	It("drops when permission denied", func() {
		vali.allowed = false
		in <- &types.UMHMessage{Content: encodeAction("deploy"), Email: "user@test.com"}
		Consistently(g.VerifiedInboundChan()).ShouldNot(Receive())
		Expect(vali.callCount).To(Equal(1))
	})

	It("drops invalid content", func() {
		in <- &types.UMHMessage{Content: "not-valid-base64!!!", Email: "user@test.com"}
		Consistently(g.VerifiedInboundChan()).ShouldNot(Receive())
	})

	It("exits cleanly on context cancel", func() {
		cancel()
		g.Stop()
		Expect(g.running).To(BeFalse())
	})
})
