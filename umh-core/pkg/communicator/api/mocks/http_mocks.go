package mocks

import (
	"net/http"

	http2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/shared/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/fail"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/tools/safejson"

	"github.com/google/uuid"
	"github.com/h2non/gock"
)

func MockLogin() {
	gock.InterceptClient(http2.GetClient(false))
	gock.New("https://management.umh.app").
		Post("/api/v2/instance/login").
		AddMatcher(func(request *http.Request, request2 *gock.Request) (bool, error) {
			return request.Header.Get("Authorization") != "", nil
		}).
		ReplyFunc(func(response *gock.Response) {
			response.Cookies = append(response.Cookies, &http.Cookie{
				Name:  "token",
				Value: "SOME_JWT",
			})

			// Encode the JSON response body
			body := map[string]interface{}{
				"uuid": uuid.New(),
				"name": "SUCH_NAME",
			}
			bodyBytes, err := safejson.Marshal(body)
			if err != nil {
				fail.ErrorBatchedf("Failed to marshal the body: %v", err)
				response.StatusCode = 500
			}

			// Set the response body
			response.BodyString(string(bodyBytes))
			response.StatusCode = 200
			response.SetHeader("Set-Cookie", "token=SOME_JWT")
		})
}
func MockSubscribeMessage() {
	gock.InterceptClient(http2.GetClient(false))
	message, err := encoding.EncodeMessageFromUserToUMHInstance(models.UMHMessageContent{MessageType: models.Subscribe, Payload: ""})
	if err != nil {
		fail.Fatalf("Failed to encrypt message: %v", err)
	}
	gock.New("https://management.umh.app").
		Get("/api/v2/instance/pull").
		Reply(200).
		JSON(backend_api_structs.PullPayload{UMHMessages: []models.UMHMessage{
			{
				InstanceUUID: uuid.Nil,
				Email:        "some-email@umh.app",
				Content:      message,
			},
		}})
}

func MockSubscribeMessageWithPayload(payload backend_api_structs.PullPayload) {
	gock.InterceptClient(http2.GetClient(false))
	gock.New("https://management.umh.app").
		Get("/api/v2/instance/pull").
		Reply(200).
		JSON(payload)
}

func MockPushEndpoint() {
	gock.InterceptClient(http2.GetClient(false))
	gock.New("https://management.umh.app").
		AddMatcher(func(request *http.Request, request2 *gock.Request) (bool, error) {
			_, err := request.Cookie("token")
			if err != nil {
				return false, err
			}
			return true, nil
		}).
		Post("/api/v2/instance/push").
		Reply(200)
}

func MockPushEndpointWithPayload(payload backend_api_structs.PushPayload) {
	gock.InterceptClient(http2.GetClient(false))
	gock.New("https://management.umh.app").
		AddMatcher(func(request *http.Request, request2 *gock.Request) (bool, error) {
			_, err := request.Cookie("token")
			if err != nil {
				return false, err
			}
			return true, nil
		}).
		Post("/api/v2/instance/push").
		JSON(payload).
		Reply(200)
}

func MockUserCertificateEndpoint() {
	gock.InterceptClient(http2.GetClient(false))
	gock.New("https://management.umh.app").
		Get("/api/v2/instance/user/certificate").
		MatchParam("email", "some-email@umh.app").
		Reply(204)
}
