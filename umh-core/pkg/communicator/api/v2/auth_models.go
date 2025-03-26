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
