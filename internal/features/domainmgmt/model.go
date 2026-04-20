package domainmgmt

import (
	"time"

	"github.com/google/uuid"
)

// Model represents a domain record.
type Model struct {
	ID                     uuid.UUID  `json:"id"`
	UserID                 uuid.UUID  `json:"user_id"`
	Domain                 string     `json:"domain"`
	Status                 string     `json:"status"`
	SPFRecord              *string    `json:"spf_record"`
	DKIMPublicKey          *string    `json:"dkim_public_key"`
	DKIMPrivateKeyEncrypted []byte    `json:"-"`
	DKIMSelector           *string    `json:"dkim_selector"`
	DMARCRecord            *string    `json:"dmarc_record"`
	BIMILogoURL            *string    `json:"bimi_logo_url"`
	BIMIVmcURL             *string    `json:"bimi_vmc_url"`
	MXVerified             bool       `json:"mx_verified"`
	WarmupDay              int        `json:"warmup_day"`
	WarmupActive           bool       `json:"warmup_active"`
	LastVerifiedAt         *time.Time `json:"last_verified_at"`
	DKIMRotatedAt          *time.Time `json:"dkim_rotated_at"`
	CreatedAt              time.Time  `json:"created_at"`
}

// SenderEmail represents a sender email record.
type SenderEmail struct {
	ID                uuid.UUID  `json:"id"`
	UserID            uuid.UUID  `json:"user_id"`
	DomainID          uuid.UUID  `json:"domain_id"`
	Email             string     `json:"email"`
	Verified          bool       `json:"verified"`
	VerificationToken *string    `json:"-"`
	VerifiedAt        *time.Time `json:"verified_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// AddDomainRequest is the request to add a new domain.
type AddDomainRequest struct {
	Domain     string  `json:"domain" validate:"required"`
	BIMILogo   *string `json:"bimi_logo_url,omitempty"`
	BIMIVmc    *string `json:"bimi_vmc_url,omitempty"`
}

// DomainResponse is the API response for a domain.
type DomainResponse struct {
	ID             uuid.UUID  `json:"id"`
	Domain         string     `json:"domain"`
	Status         string     `json:"status"`
	SPFRecord      *string    `json:"spf_record"`
	DKIMSelector   *string    `json:"dkim_selector"`
	DKIMPublicKey  *string    `json:"dkim_public_key"`
	DMARCRecord    *string    `json:"dmarc_record"`
	MXVerified     bool       `json:"mx_verified"`
	DNSRecords     []DNSRecord `json:"dns_records"`
	CreatedAt      time.Time  `json:"created_at"`
}

// DNSRecord is a DNS record the user needs to add.
type DNSRecord struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	Status string `json:"status"` // pending, verified
}
