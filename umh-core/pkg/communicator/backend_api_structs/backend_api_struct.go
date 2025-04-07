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
	Email            string  `json:"email" binding:"required"`
	Password         string  `json:"password" binding:"required"`
	FirstName        string  `json:"firstName" binding:"required"`
	LastName         string  `json:"lastName" binding:"required"`
	CompanyName      string  `json:"company" binding:"required"`
	Certificate      *string `json:"certificate"`
	EncryptedPrivKey *string `json:"encryptedPrivateKey"`
	CompanyCert      *string `json:"companyCertificate"`
	CompanyPrivKey   *string `json:"companyEncryptedPrivateKey"`
}

type CompanyUserRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// UpdateInstancePayload is the payload for updating an instance
type UpdateInstancePayload struct {
	InstanceName string `json:"instance_name"`
}

type RegisterInstancePayload struct {
	InstanceName      string    `json:"instanceName" binding:"required,max=100"`
	HashHashAuthToken string    `json:"hashHashAuthToken" binding:"required,len=64"`
	InstanceUUID      uuid.UUID `json:"instanceUUID" binding:"required"`
	Certificate       *string   `json:"certificate"`
	EncryptedPrivKey  *string   `json:"encryptedPrivateKey"`
}

type PushPayload struct {
	UMHMessages []models.UMHMessage `json:"UMHMessages"`
}

type PullPayload struct {
	UMHMessages []models.UMHMessage `json:"UMHMessages"`
}

// CompanyDetails contains detailed information about a company
type CompanyDetails struct {
	Name             string        `json:"name"`
	LicenseStatus    LicenseStatus `json:"licenseStatus"`
	UserCount        int64         `json:"userCount"`
	Owner            *string       `json:"owner"`
	Certificate      *string       `json:"certificate"`
	EncryptedPrivKey *string       `json:"encryptedPrivateKey"`
}

type LicenseStatus struct {
	IsActive    bool   `json:"isActive"`
	ValidTo     string `json:"validTo"`
	Description string `json:"description"`
}

// UserMeResponse contains the user's personal information and company details
type UserMeResponse struct {
	Email            string         `json:"email"`
	FirstName        string         `json:"firstName"`
	LastName         string         `json:"lastName"`
	Certificate      *string        `json:"certificate"`
	EncryptedPrivKey *string        `json:"encryptedPrivateKey"`
	CompanyDetails   CompanyDetails `json:"companyDetails"`
}

type UserLoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UserLoginResponse struct {
	Email            string         `json:"email"`
	FirstName        string         `json:"firstName"`
	LastName         string         `json:"lastName"`
	Certificate      *string        `json:"certificate"`
	EncryptedPrivKey *string        `json:"encryptedPrivateKey"`
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
	UUID             string         `json:"uuid"`
	Name             string         `json:"name"`
	Certificate      *string        `json:"certificate"`
	EncryptedPrivKey *string        `json:"encryptedPrivateKey"`
	CompanyDetails   CompanyDetails `json:"companyDetails"`
}

// Location represents a location in a hierarchy
type Location struct {
	Enterprise     string `json:"enterprise"`
	Site           string `json:"site"`
	Area           string `json:"area"`
	ProductionLine string `json:"productionLine"`
	WorkCell       string `json:"workCell"`
}

// CreateInviteRequest is the payload for creating a new invite
type CreateInviteRequest struct {
	FirstName        string     `json:"firstName" binding:"required"`
	LastName         string     `json:"lastName" binding:"required"`
	Email            string     `json:"email" binding:"required,email"`
	Role             string     `json:"role" binding:"required"`
	Hierarchies      []Location `json:"hierarchies"`
	Certificate      string     `json:"certificate" binding:"required"`
	EncryptedPrivKey string     `json:"encryptedPrivateKey" binding:"required"`
}

// InviteResponse represents an invite in API responses
type InviteResponse struct {
	InviteID  string    `json:"inviteId"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// RemoveUserRequest is the payload for removing a user from a company
type RemoveUserRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// UserResponse represents a user in API responses
type UserResponse struct {
	Email     string    `json:"email"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	IsOwner   bool      `json:"isOwner"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// RedeemInviteRequest is the payload for redeeming an invite
type RedeemInviteRequest struct {
	InviteID            string `json:"inviteId" binding:"required"`
	EncryptedPrivateKey string `json:"encryptedPrivateKey" binding:"required"`
	Password            string `json:"password" binding:"required"`
}

// UploadCertificatePatchRequest is the payload for uploading a certificate patch
type UploadCertificatePatchRequest struct {
	UserEmail           string `json:"userEmail" binding:"required,email"`
	Certificate         string `json:"certificate" binding:"required"`
	EncryptedPrivateKey string `json:"encryptedPrivateKey" binding:"required"`
}

// CertificatePatchResponse is the response for retrieving a certificate patch
type CertificatePatchResponse struct {
	HasPatch            bool      `json:"hasPatch"`
	Certificate         string    `json:"certificate,omitempty"`
	EncryptedPrivateKey string    `json:"encryptedPrivateKey,omitempty"`
	CreatedAt           time.Time `json:"createdAt,omitempty"`
	UpdatedAt           time.Time `json:"updatedAt,omitempty"`
}

// ApplyCertificatePatchRequest is the request for applying a certificate patch
type ApplyCertificatePatchRequest struct {
	Certificate         string `json:"certificate" binding:"required"`
	EncryptedPrivateKey string `json:"encryptedPrivateKey" binding:"required"`
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

// RequestUserCertificateResponse is the response payload for a user certificate request
type RequestUserCertificateResponse struct {
	UserEmail   string `json:"userEmail"`
	Certificate string `json:"certificate"`
}
