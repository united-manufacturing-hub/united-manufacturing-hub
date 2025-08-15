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

package mocks

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/h2non/gock"
	http2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/safejson"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"go.uber.org/zap"
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
				sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to marshal the body: %v", err)

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
		sentry.ReportIssuef(sentry.IssueTypeError, zap.S(), "Failed to encrypt message: %v", err)
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
