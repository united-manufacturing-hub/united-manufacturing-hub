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

package v2

import "github.com/google/uuid"

type LoginResponse struct {
	JWT                   string                         `json:"jwt"`
	UUID                  uuid.UUID                      `json:"uuid"`
	Name                  string                         `json:"name"`
	CertificateAndPrivKey *InstanceCertificateAndPrivKey `json:"certificate_and_priv_key"`
	CompanyCertificate    *InstanceCompanyCertificate    `json:"company_certificate"`
}

type InstanceCertificateAndPrivKey struct {
	Certificate      string `json:"certificate"`
	EncryptedPrivKey string `json:"encrypted_priv_key"`
	DerivedPassword  string `json:"derived_password"`
}

type InstanceCompanyCertificate struct {
	Certificate string `json:"certificate"`
}
