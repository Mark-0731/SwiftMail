package dkim

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"strings"
)

// KeyPair holds a generated DKIM RSA-2048 key pair.
type KeyPair struct {
	PrivateKey    *rsa.PrivateKey
	PrivateKeyPEM string
	PublicKeyPEM  string
	DNSRecord     string // The TXT record value for DNS
	Selector      string
}

// GenerateKeyPair generates an RSA-2048 key pair for DKIM signing.
func GenerateKeyPair(selector string) (*KeyPair, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode private key to PEM
	privKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privKeyBytes,
	})

	// Encode public key to PEM
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	})

	// Generate DNS TXT record value
	pubKeyBase64 := base64.StdEncoding.EncodeToString(pubKeyBytes)
	dnsRecord := fmt.Sprintf("v=DKIM1; k=rsa; p=%s", pubKeyBase64)

	return &KeyPair{
		PrivateKey:    privateKey,
		PrivateKeyPEM: string(privKeyPEM),
		PublicKeyPEM:  string(pubKeyPEM),
		DNSRecord:     dnsRecord,
		Selector:      selector,
	}, nil
}

// ParsePrivateKey parses a PEM-encoded RSA private key.
func ParsePrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return privateKey, nil
}

// Sign generates a DKIM-Signature header for the given email content.
func Sign(privateKey *rsa.PrivateKey, domain, selector string, headers map[string]string, body string) (string, error) {
	// Canonicalize headers (relaxed)
	var signedHeaders []string
	var headerData strings.Builder

	coreHeaders := []string{"from", "to", "subject", "date", "message-id"}
	for _, h := range coreHeaders {
		if val, ok := headers[h]; ok {
			headerData.WriteString(fmt.Sprintf("%s:%s\r\n", h, strings.TrimSpace(val)))
			signedHeaders = append(signedHeaders, h)
		}
	}

	// Hash the body (simple canonicalization)
	bodyHash := hashSHA256([]byte(body))
	bodyHashB64 := base64.StdEncoding.EncodeToString(bodyHash)

	// Build DKIM-Signature header (without b= value)
	dkimHeader := fmt.Sprintf(
		"v=1; a=rsa-sha256; c=relaxed/simple; d=%s; s=%s; h=%s; bh=%s; b=",
		domain, selector, strings.Join(signedHeaders, ":"), bodyHashB64,
	)

	// Add DKIM header to data to sign
	headerData.WriteString(fmt.Sprintf("dkim-signature:%s", dkimHeader))

	// Sign with RSA-SHA256
	hashed := hashSHA256([]byte(headerData.String()))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed)
	if err != nil {
		return "", fmt.Errorf("failed to sign: %w", err)
	}

	sigB64 := base64.StdEncoding.EncodeToString(signature)
	return fmt.Sprintf("DKIM-Signature: %s%s", dkimHeader, sigB64), nil
}

func hashSHA256(data []byte) []byte {
	h := crypto.SHA256.New()
	io.WriteString(h, string(data))
	return h.Sum(nil)
}
