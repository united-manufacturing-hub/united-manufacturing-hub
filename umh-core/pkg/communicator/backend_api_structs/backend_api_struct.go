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

package backend_api_structs

import (
	"time"

	"github.com/google/uuid"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

type UserSignupRequest struct {
	Certificate      *string `json:"certificate"`
	EncryptedPrivKey *string `json:"encryptedPrivateKey"`
	CompanyCert      *string `json:"companyCertificate"`
	CompanyPrivKey   *string `json:"companyEncryptedPrivateKey"`
	Email            string  `binding:"required"                json:"email"`
	Password         string  `binding:"required"                json:"password"`
	FirstName        string  `binding:"required"                json:"firstName"`
	LastName         string  `binding:"required"                json:"lastName"`
	CompanyName      string  `binding:"required"                json:"company"`
}

type CompanyUserRequest struct {
	Email string `binding:"required,email" json:"email"`
}

// UpdateInstancePayload is the payload for updating an instance.
type UpdateInstancePayload struct {
	InstanceName string `json:"instance_name"`
}

type RegisterInstancePayload struct {
	Certificate       *string   `json:"certificate"`
	EncryptedPrivKey  *string   `json:"encryptedPrivateKey"`
	InstanceName      string    `binding:"required,max=100" json:"instanceName"`
	HashHashAuthToken string    `binding:"required,len=64"  json:"hashHashAuthToken"`
	InstanceUUID      uuid.UUID `binding:"required"         json:"instanceUUID"`
}

type PushPayload struct {
	UMHMessages []models.UMHMessage `json:"UMHMessages"`
}

type PullPayload struct {
	UMHMessages []models.UMHMessage `json:"UMHMessages"`
}

// CompanyDetails contains detailed information about a company.
type CompanyDetails struct {
	Owner            *string       `json:"owner"`
	Certificate      *string       `json:"certificate"`
	EncryptedPrivKey *string       `json:"encryptedPrivateKey"`
	Name             string        `json:"name"`
	LicenseStatus    LicenseStatus `json:"licenseStatus"`
	UserCount        int64         `json:"userCount"`
}

type LicenseStatus struct {
	ValidTo     string `json:"validTo"`
	Description string `json:"description"`
	IsActive    bool   `json:"isActive"`
}

// UserMeResponse contains the user's personal information and company details.
type UserMeResponse struct {
	Certificate      *string        `json:"certificate"`
	EncryptedPrivKey *string        `json:"encryptedPrivateKey"`
	Email            string         `json:"email"`
	FirstName        string         `json:"firstName"`
	LastName         string         `json:"lastName"`
	CompanyDetails   CompanyDetails `json:"companyDetails"`
}

type UserLoginRequest struct {
	Email    string `binding:"required" json:"email"`
	Password string `binding:"required" json:"password"`
}

type UserLoginResponse struct {
	Certificate      *string        `json:"certificate"`
	EncryptedPrivKey *string        `json:"encryptedPrivateKey"`
	Email            string         `json:"email"`
	FirstName        string         `json:"firstName"`
	LastName         string         `json:"lastName"`
	CompanyDetails   CompanyDetails `json:"companyDetails"`
}

type DemoRequestPayload struct {
	Enabled bool `json:"enabled"`
}

type DemoResponsePayload struct {
	TestCertificatePublic  string `json:"test_certificate_public"`
	TestCertificatePrivate string `json:"test_certificate_private"`
}

type DeleteInstancePayload struct {
	InstanceUUID uuid.UUID `json:"instance_uuid"`
}

type DemoInstanceResponsePayload struct {
	TestCertificatePublic  string `json:"test_certificate_public"`
	TestCertificatePrivate string `json:"test_certificate_private"`
}

type InstanceLoginResponse struct {
	Certificate      *string        `json:"certificate"`
	EncryptedPrivKey *string        `json:"encryptedPrivateKey"`
	UUID             string         `json:"uuid"`
	Name             string         `json:"name"`
	CompanyDetails   CompanyDetails `json:"companyDetails"`
}

// Location represents a location in a hierarchy.
type Location struct {
	Enterprise     string `json:"enterprise"`
	Site           string `json:"site"`
	Area           string `json:"area"`
	ProductionLine string `json:"productionLine"`
	WorkCell       string `json:"workCell"`
}

// CreateInviteRequest is the payload for creating a new invite.
type CreateInviteRequest struct {
	FirstName        string     `binding:"required"       json:"firstName"`
	LastName         string     `binding:"required"       json:"lastName"`
	Email            string     `binding:"required,email" json:"email"`
	Role             string     `binding:"required"       json:"role"`
	Certificate      string     `binding:"required"       json:"certificate"`
	EncryptedPrivKey string     `binding:"required"       json:"encryptedPrivateKey"`
	Hierarchies      []Location `json:"hierarchies"`
}

// InviteResponse represents an invite in API responses.
type InviteResponse struct {
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	InviteID  string    `json:"inviteId"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Email     string    `json:"email"`
}

// RemoveUserRequest is the payload for removing a user from a company.
type RemoveUserRequest struct {
	Email string `binding:"required,email" json:"email"`
}

// UserResponse represents a user in API responses.
type UserResponse struct {
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Email     string    `json:"email"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	IsOwner   bool      `json:"isOwner"`
}

// RedeemInviteRequest is the payload for redeeming an invite.
type RedeemInviteRequest struct {
	InviteID            string `binding:"required" json:"inviteId"`
	EncryptedPrivateKey string `binding:"required" json:"encryptedPrivateKey"`
	Password            string `binding:"required" json:"password"`
}

// UploadCertificatePatchRequest is the payload for uploading a certificate patch.
type UploadCertificatePatchRequest struct {
	UserEmail           string `binding:"required,email" json:"userEmail"`
	Certificate         string `binding:"required"       json:"certificate"`
	EncryptedPrivateKey string `binding:"required"       json:"encryptedPrivateKey"`
}

// CertificatePatchResponse is the response for retrieving a certificate patch.
type CertificatePatchResponse struct {
	CreatedAt           time.Time `json:"createdAt,omitempty"`
	UpdatedAt           time.Time `json:"updatedAt,omitempty"`
	Certificate         string    `json:"certificate,omitempty"`
	EncryptedPrivateKey string    `json:"encryptedPrivateKey,omitempty"`
	HasPatch            bool      `json:"hasPatch"`
}

// ApplyCertificatePatchRequest is the request for applying a certificate patch.
type ApplyCertificatePatchRequest struct {
	Certificate         string `binding:"required" json:"certificate"`
	EncryptedPrivateKey string `binding:"required" json:"encryptedPrivateKey"`
}

type InviteDetailsResponse struct {
	InviteID            string `json:"inviteId"`
	FirstName           string `json:"firstName"`
	LastName            string `json:"lastName"`
	Email               string `json:"email"`
	Certificate         string `json:"certificate"`
	EncryptedPrivateKey string `json:"encryptedPrivateKey"`
	CompanyName         string `json:"companyName"`
	CompanyOwner        string `json:"companyOwner"`
}

// Note: RequestUserCertificateRequest is no longer used as we now use query parameters
// instead of a JSON body for the GET /instance/user/certificate endpoint

// RequestUserCertificateResponse is the response payload for a user certificate request.
type RequestUserCertificateResponse struct {
	UserEmail   string `json:"userEmail"`
	Certificate string `json:"certificate"`
}
