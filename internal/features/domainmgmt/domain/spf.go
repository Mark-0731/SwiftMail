package domain

import "fmt"

// GenerateSPFRecord generates an SPF TXT record for a domain.
func GenerateSPFRecord(sendingIPs []string) string {
	record := "v=spf1"
	for _, ip := range sendingIPs {
		record += fmt.Sprintf(" ip4:%s", ip)
	}
	record += " include:_spf.swiftmail.dev ~all"
	return record
}

// GenerateDMARCRecord generates a DMARC TXT record.
func GenerateDMARCRecord(domain string) string {
	return fmt.Sprintf(
		"v=DMARC1; p=quarantine; rua=mailto:dmarc-reports@%s; ruf=mailto:dmarc-forensics@%s; fo=1; adkim=s; aspf=s; pct=100",
		domain, domain,
	)
}

// SPFRecordName returns the DNS name for the SPF record.
func SPFRecordName(domain string) string {
	return domain
}

// DKIMRecordName returns the DNS name for the DKIM record.
func DKIMRecordName(selector, domain string) string {
	return fmt.Sprintf("%s._domainkey.%s", selector, domain)
}

// DMARCRecordName returns the DNS name for the DMARC record.
func DMARCRecordName(domain string) string {
	return fmt.Sprintf("_dmarc.%s", domain)
}
