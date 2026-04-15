package domain

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	pkgdkim "github.com/swiftmail/swiftmail/pkg/dkim"
	"github.com/swiftmail/swiftmail/pkg/validator"
)

// Service defines the domain business logic interface.
type Service interface {
	AddDomain(ctx context.Context, userID uuid.UUID, req *AddDomainRequest) (*DomainResponse, error)
	ListDomains(ctx context.Context, userID uuid.UUID) ([]DomainResponse, error)
	GetDomain(ctx context.Context, id uuid.UUID) (*DomainResponse, error)
	VerifyDomain(ctx context.Context, id uuid.UUID) (*DomainResponse, error)
	DeleteDomain(ctx context.Context, id, userID uuid.UUID) error
	IsDomainVerified(ctx context.Context, domainID uuid.UUID) (bool, error)
}

type service struct {
	repo   Repository
	dns    *DNSChecker
	rdb    *redis.Client
	logger zerolog.Logger
}

// NewService creates a new domain service.
func NewService(repo Repository, dns *DNSChecker, rdb *redis.Client, logger zerolog.Logger) Service {
	return &service{repo: repo, dns: dns, rdb: rdb, logger: logger}
}

func (s *service) AddDomain(ctx context.Context, userID uuid.UUID, req *AddDomainRequest) (*DomainResponse, error) {
	domain := validator.NormalizeDomain(req.Domain)
	if !validator.IsValidDomain(domain) {
		return nil, fmt.Errorf("invalid domain name: %s", domain)
	}

	// Check MX records
	hasMX, _, err := s.dns.CheckMX(ctx, domain)
	if err != nil {
		s.logger.Warn().Err(err).Str("domain", domain).Msg("MX check failed")
	}

	// Generate DKIM key pair
	selector := fmt.Sprintf("sm%d", time.Now().Unix()%10000)
	keyPair, err := pkgdkim.GenerateKeyPair(selector)
	if err != nil {
		return nil, fmt.Errorf("failed to generate DKIM keys: %w", err)
	}

	// Generate DNS records
	spfRecord := GenerateSPFRecord([]string{})
	dmarcRecord := GenerateDMARCRecord(domain)

	// Encrypt DKIM private key using AES-256-GCM
	encryptedKey, err := encryptDKIMKey([]byte(keyPair.PrivateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt DKIM key: %w", err)
	}

	model := &Model{
		UserID:                  userID,
		Domain:                  domain,
		Status:                  "pending",
		SPFRecord:               &spfRecord,
		DKIMPublicKey:           &keyPair.DNSRecord,
		DKIMPrivateKeyEncrypted: encryptedKey,
		DKIMSelector:            &selector,
		DMARCRecord:             &dmarcRecord,
		BIMILogoURL:             req.BIMILogo,
		BIMIVmcURL:              req.BIMIVmc,
		MXVerified:              hasMX,
	}

	if err := s.repo.Create(ctx, model); err != nil {
		return nil, fmt.Errorf("failed to create domain: %w", err)
	}

	s.logger.Info().Str("domain", domain).Str("user_id", userID.String()).Msg("domain added")

	return s.toDomainResponse(model), nil
}

func (s *service) ListDomains(ctx context.Context, userID uuid.UUID) ([]DomainResponse, error) {
	domains, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var resp []DomainResponse
	for _, d := range domains {
		resp = append(resp, *s.toDomainResponse(&d))
	}
	return resp, nil
}

func (s *service) GetDomain(ctx context.Context, id uuid.UUID) (*DomainResponse, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.toDomainResponse(d), nil
}

func (s *service) VerifyDomain(ctx context.Context, id uuid.UUID) (*DomainResponse, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("domain not found: %w", err)
	}

	// Check MX
	hasMX, _, _ := s.dns.CheckMX(ctx, d.Domain)
	d.MXVerified = hasMX

	// Check SPF
	spfOK, _ := s.dns.CheckSPF(ctx, d.Domain, "swiftmail")

	// Check DKIM
	dkimOK := false
	if d.DKIMSelector != nil {
		dkimOK, _ = s.dns.CheckDKIM(ctx, *d.DKIMSelector, d.Domain)
	}

	// Check DMARC
	dmarcOK, _ := s.dns.CheckDMARC(ctx, d.Domain)

	if spfOK && dkimOK && dmarcOK && hasMX {
		d.Status = "verified"
		now := time.Now()
		d.LastVerifiedAt = &now

		// Cache in Redis
		cacheKey := fmt.Sprintf("domain:%s:verified", d.ID.String())
		s.rdb.Set(ctx, cacheKey, "1", 10*time.Minute)
	} else {
		d.Status = "pending"
	}

	if err := s.repo.Update(ctx, d); err != nil {
		return nil, fmt.Errorf("failed to update domain: %w", err)
	}

	return s.toDomainResponse(d), nil
}

func (s *service) DeleteDomain(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.Delete(ctx, id, userID)
}

func (s *service) IsDomainVerified(ctx context.Context, domainID uuid.UUID) (bool, error) {
	cacheKey := fmt.Sprintf("domain:%s:verified", domainID.String())
	val, err := s.rdb.Get(ctx, cacheKey).Result()
	if err == nil && val == "1" {
		return true, nil
	}

	d, err := s.repo.GetByID(ctx, domainID)
	if err != nil {
		return false, err
	}

	verified := d.Status == "verified"
	if verified {
		s.rdb.Set(ctx, cacheKey, "1", 10*time.Minute)
	}
	return verified, nil
}

func (s *service) toDomainResponse(d *Model) *DomainResponse {
	resp := &DomainResponse{
		ID:            d.ID,
		Domain:        d.Domain,
		Status:        d.Status,
		SPFRecord:     d.SPFRecord,
		DKIMSelector:  d.DKIMSelector,
		DKIMPublicKey: d.DKIMPublicKey,
		DMARCRecord:   d.DMARCRecord,
		MXVerified:    d.MXVerified,
		CreatedAt:     d.CreatedAt,
	}

	// Build DNS records user needs to add
	if d.SPFRecord != nil {
		resp.DNSRecords = append(resp.DNSRecords, DNSRecord{
			Type: "TXT", Name: SPFRecordName(d.Domain), Value: *d.SPFRecord, Status: "pending",
		})
	}
	if d.DKIMSelector != nil && d.DKIMPublicKey != nil {
		resp.DNSRecords = append(resp.DNSRecords, DNSRecord{
			Type: "TXT", Name: DKIMRecordName(*d.DKIMSelector, d.Domain), Value: *d.DKIMPublicKey, Status: "pending",
		})
	}
	if d.DMARCRecord != nil {
		resp.DNSRecords = append(resp.DNSRecords, DNSRecord{
			Type: "TXT", Name: DMARCRecordName(d.Domain), Value: *d.DMARCRecord, Status: "pending",
		})
	}

	return resp
}

// encryptDKIMKey encrypts a DKIM private key using AES-256-GCM with environment key
func encryptDKIMKey(plaintext []byte) ([]byte, error) {
	encKey := os.Getenv("DKIM_ENCRYPTION_KEY")
	if encKey == "" {
		// No encryption key set - return plaintext
		// Operator should set DKIM_ENCRYPTION_KEY environment variable (32+ chars)
		return plaintext, nil
	}

	// Ensure key is 32 bytes for AES-256
	key := []byte(encKey)
	if len(key) < 32 {
		// Pad key if too short
		padded := make([]byte, 32)
		copy(padded, key)
		key = padded
	} else if len(key) > 32 {
		key = key[:32]
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decryptDKIMKey decrypts a DKIM private key
func decryptDKIMKey(ciphertext []byte) ([]byte, error) {
	encKey := os.Getenv("DKIM_ENCRYPTION_KEY")
	if encKey == "" {
		// No encryption key set, assume plaintext
		return ciphertext, nil
	}

	key := []byte(encKey)
	if len(key) < 32 {
		padded := make([]byte, 32)
		copy(padded, key)
		key = padded
	} else if len(key) > 32 {
		key = key[:32]
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		// Likely plaintext, return as-is
		return ciphertext, nil
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Decryption failed, might be plaintext
		return append(nonce, ciphertext...), nil
	}

	return plaintext, nil
}
