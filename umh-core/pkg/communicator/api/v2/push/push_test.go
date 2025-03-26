package push_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"

	"github.com/google/uuid"
	"github.com/h2non/gock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/mocks"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/helper"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
)

var _ = Describe("Pusher", Label("msgpush"), func() {

	var server *ghttp.Server
	var dog watchdog.Iface
	var pusher *push.Pusher
	var outboundChannel chan *models.UMHMessage
	var deadLetterChan chan push.DeadLetter
	var instanceUUID uuid.UUID
	var backoff *tools.Backoff

	BeforeEach(func() {
		Skip("This test is failing / panicing because of 'Watchdog context done' as well as 'panic: send on closed channel'. ENG-1470 is created to fix this issue.")

		server = ghttp.NewServer()
		err := os.Setenv("API_URL", server.URL())
		Expect(err).ToNot(HaveOccurred())
		dog = watchdog.NewFakeWatchdog()
		instanceUUID = uuid.New()
		backoff = tools.NewBackoff(1*time.Millisecond, 1*time.Nanosecond, 1*time.Second, tools.BackoffPolicyLinear)
	})

	JustBeforeEach(func() {
		outboundChannel = make(chan *models.UMHMessage, 10)
		deadLetterChan = push.DefaultDeadLetterChanBuffer()
		pusher = push.NewPusher(instanceUUID, "jwt", dog, outboundChannel, deadLetterChan, backoff, false)
		Expect(pusher).ToNot(BeNil())

	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		//	close(outboundChannel)
		//	close(deadLetterChan)
	})

	Describe("Pushing Messages to the Backend", func() {

		Context("when creating a new pusher", func() {
			It("should return a new pusher", func() {
				pusher := push.NewPusher(uuid.New(), "jwt", dog, outboundChannel, deadLetterChan, backoff, false)
				Expect(pusher).ToNot(BeNil())
			})
		})

		Context("when a new message is pushed", func() {
			It("should push the message to the outbound channel", func() {
				message := models.UMHMessage{
					InstanceUUID: instanceUUID,
					Email:        "test@umh.app",
					Content:      "test",
				}
				pusher.Push(message)
				Eventually(outboundChannel).Should(Receive(Equal(&message)))

			})
		})

		Context("when a message is pushed to the outbound channel", Label("backend_call"), func() {
			JustBeforeEach(func() {
				server.AppendHandlers(
					ghttp.VerifyRequest("POST", "/v2/instance/push"),
				)
				pusher.Start()
			})
			It("backend server should receive the message", func() {
				message := models.UMHMessage{
					InstanceUUID: instanceUUID,
					Email:        "test@umh.app",
					Content:      "test",
				}
				pusher.Push(message)

				Eventually(func() int {
					return len(server.ReceivedRequests())
				}, 10*time.Second).Should(Equal(1))
				Eventually(deadLetterChan).Should(BeEmpty())
			})
		})

		Context("backend server returns an error", Label("backend_call", "backend_error"), func() {
			JustBeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/v2/instance/push"),
						ghttp.RespondWith(500, nil),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/v2/instance/push"),
						ghttp.RespondWith(200, nil),
					),
				)
				pusher.Start()
			})
			When("when a message is pushed to the outbound channel", func() {
				It("should push the message to the dead letter channel and retry server calls", func() {
					message := models.UMHMessage{
						InstanceUUID: instanceUUID,
						Email:        "test@umh.app",
						Content:      "test",
					}
					pusher.Push(message)
					Eventually(func() int {
						return len(server.ReceivedRequests())
					}, 10*time.Second).Should(Equal(2))
					Eventually(deadLetterChan).Should(BeEmpty())
				})
			})
		})

		Context("backend server returns an error", Label("backend_call", "backend_error_404"), func() {
			JustBeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/v2/instance/push"),
						ghttp.RespondWith(400, nil),
					),
				)
				pusher.Start()
			})
			When("when a message is pushed to the outbound channel and server returns 400", func() {
				It("then message should not be retried in the deadletter channel", func() {
					message := models.UMHMessage{
						InstanceUUID: instanceUUID,
						Email:        "test@umh.app",
						Content:      "test",
					}
					pusher.Push(message)
					Eventually(func() int {
						return len(server.ReceivedRequests())
					}, 10*time.Second).Should(Equal(1))
				})
			})
		})

		Context("backend server returns an error", Label("backend_call", "backend_error", "backend_error_503"), func() {
			JustBeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/v2/instance/push"),
						ghttp.RespondWith(503, nil),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/v2/instance/push"),
						ghttp.RespondWith(503, nil),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/v2/instance/push"),
						ghttp.RespondWith(503, nil),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/v2/instance/push"),
						ghttp.RespondWith(503, nil),
					),
				)
				pusher.Start()
			})
			When("when a message is pushed to the outbound channel", func() {
				It("should retry server call for 3 times", func() {
					message := models.UMHMessage{
						InstanceUUID: instanceUUID,
						Email:        "test@umh.app",
						Content:      "test",
					}
					pusher.Push(message)
					// 4 calls in total, 1 initial call and 3 retries
					Eventually(func() int {
						return len(server.ReceivedRequests())
					}, 10*time.Second).Should(Equal(4))
					Eventually(deadLetterChan).Should(BeEmpty())
				})
			})
		})
	})
})

var _ = Describe("Requester", func() {
	var state *appstate.ApplicationState
	var cncl context.CancelFunc
	BeforeEach(func() {
		By("setting the DEMO_MODE environment variable")
		// Set DEMO_MODE to true in order to use the fake kube clientset in this suite
		// instead of a real one
		helper.Setenv("DEMO_MODE", "true")

		By("setting the API_URL environment variable")
		// Set API_URL to the production API URL in order for the gock library to correctly
		// intercept the requests to the production API done by the code under test
		helper.Setenv("API_URL", "https://management.umh.app/api")
		state, cncl, _ = appstate.NewTestAppState()
		state.SetLoginResponse(&models2.LoginResponse{JWT: "valid-jwt"})
		state.InitialiseAndStartPusher()
		// This will panic once we cancel the context, but it's fine for the tests
		go func() {
			// Defer recover to avoid the panic from crashing the tests
			defer func() {
				if r := recover(); r != nil {
					// Check if the panic was caused by the context being canceled
					if strings.Contains(r.(string), "context done") {
						return
					}
					panic(r)
				}
			}()
			state.StartWatchdog()
		}()
	})

	AfterEach(func() {
		cncl()
	})

	// The tests in this context are ported from the old push_test.go file
	Context("Push", func() {
		var message models.UMHMessage

		BeforeEach(func() {
			By("Mocking the push endpoint with a status message payload")
			statusMessage := models.StatusMessageV2{}
			bytes, err := safejson.Marshal(statusMessage)
			Expect(err).ToNot(HaveOccurred())

			messageContent, err := encoding.EncodeMessageFromUMHInstanceToUser(models.UMHMessageContent{
				MessageType: models.Status,
				Payload:     string(bytes),
			})
			Expect(err).ToNot(HaveOccurred())

			login := state.LoginResponse
			Expect(login).ToNot(BeNil())

			message = models.UMHMessage{
				InstanceUUID: login.UUID,
				Email:        "some-email@umh.app",
				Content:      messageContent,
			}

			mocks.MockPushEndpointWithPayload(backend_api_structs.PushPayload{
				UMHMessages: []models.UMHMessage{message},
			}) // no need to defer gock.Off, as it is done in the cleanup
		})

		AfterEach(func() {
			gock.Off()
		})

		It("should return a response for a valid token", func() {
			Expect(len(state.OutboundChannel)).To(BeZero(), "the channel should be empty", len(state.OutboundChannel))
			state.Pusher.Push(message)
			Expect(len(state.OutboundChannel)).To(Equal(1), "the message should be added to the channel", len(state.OutboundChannel))

			Eventually(func() int {
				return len(state.OutboundChannel)
			}, 10*time.Second).Should(BeZero(), "the message should be removed from the channel", len(state.OutboundChannel))

			// The only way to check if the request was made is to check if there are any unmatched requests
			// At this point, gock should only have the login request and the push request, which should both be matched
			Expect(gock.HasUnmatchedRequest()).To(BeFalse(), func() string {
				// Print the unmatched requests in case of failure
				var details []string
				for _, req := range gock.GetUnmatchedRequests() {
					detail := fmt.Sprintf("\tMethod: %s, URL: %s, Header: %v, Body: %v", req.Method, req.URL.String(), req.Header, requestBodyToString(req))
					details = append(details, detail)
				}
				return fmt.Sprintf("gock should not have unmatched requests:\n%+v", strings.Join(details, "\n"))
			}())
		})
	})
})

// Helper function to convert request body to a string
func requestBodyToString(req *http.Request) string {
	if req.Body == nil {
		return "<nil>"
	}
	bodyBytes, err := io.ReadAll(req.Body)
	// Important: Reset req.Body so it can be read again
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Sprintf("error reading body: %v", err)
	}
	return string(bodyBytes)
}
