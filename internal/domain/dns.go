package domain

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// DNSChecker performs DNS lookups using Go's net package.
type DNSChecker struct{}

// NewDNSChecker creates a new DNS checker.
func NewDNSChecker() *DNSChecker {
	return &DNSChecker{}
}

// CheckMX verifies that the domain has valid MX records.
func (d *DNSChecker) CheckMX(ctx context.Context, domain string) (bool, []string, error) {
	records, err := net.LookupMX(domain)
	if err != nil {
		return false, nil, fmt.Errorf("MX lookup failed for %s: %w", domain, err)
	}

	if len(records) == 0 {
		return false, nil, nil
	}

	var hosts []string
	for _, r := range records {
		hosts = append(hosts, r.Host)
	}

	return true, hosts, nil
}

// CheckTXT retrieves TXT records for a domain and checks for a specific prefix.
func (d *DNSChecker) CheckTXT(ctx context.Context, domain, prefix string) (bool, string, error) {
	records, err := net.LookupTXT(domain)
	if err != nil {
		return false, "", nil // DNS lookup might fail, not an error
	}

	for _, r := range records {
		if strings.HasPrefix(r, prefix) {
			return true, r, nil
		}
	}

	return false, "", nil
}

// CheckSPF verifies that the domain has the expected SPF record.
func (d *DNSChecker) CheckSPF(ctx context.Context, domain, expectedInclude string) (bool, error) {
	found, record, err := d.CheckTXT(ctx, domain, "v=spf1")
	if err != nil {
		return false, err
	}

	if !found {
		return false, nil
	}

	return strings.Contains(record, expectedInclude), nil
}

// CheckDKIM verifies that the DKIM record exists at selector._domainkey.domain.
func (d *DNSChecker) CheckDKIM(ctx context.Context, selector, domain string) (bool, error) {
	dkimDomain := selector + "._domainkey." + domain
	found, _, err := d.CheckTXT(ctx, dkimDomain, "v=DKIM1")
	return found, err
}

// CheckDMARC verifies that the DMARC record exists at _dmarc.domain.
func (d *DNSChecker) CheckDMARC(ctx context.Context, domain string) (bool, error) {
	dmarcDomain := "_dmarc." + domain
	found, _, err := d.CheckTXT(ctx, dmarcDomain, "v=DMARC1")
	return found, err
}

// LookupMXRecords returns MX records for a domain, used by SMTP sender for delivery.
func (d *DNSChecker) LookupMXRecords(ctx context.Context, domain string) ([]*net.MX, error) {
	return net.LookupMX(domain)
}
