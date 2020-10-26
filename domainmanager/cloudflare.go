package domainmanager

import (
	"fmt"
	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"
	"strings"
)

type CloudflareDomainManager struct {
	zoneCache map[string]string
	cfApi     *cloudflare.API
	logger logrus.Entry
}

func (c *CloudflareDomainManager) getZoneIdFromDomain(domain string) (string, error) {
	domainParts := strings.Split(domain, ".")
	zone := fmt.Sprintf("%s.%s", domainParts[len(domainParts)-2], domainParts[len(domainParts)-1])

	if z, ok := c.zoneCache[zone]; ok {
		return z, nil
	}
	z, err := c.cfApi.ZoneIDByName(zone)
	if err == nil {
		c.zoneCache[zone] = z
	}
	return z, err
}

func (c *CloudflareDomainManager) CreateDNSRecordSingle(dnsRecord *DNSRecordSingle) error {
	zoneId, err := c.getZoneIdFromDomain(dnsRecord.Domain)
	if err != nil {
		return err
	}

	_, err = c.cfApi.CreateDNSRecord(zoneId, cloudflare.DNSRecord{
		Type:    dnsRecord.AddrType,
		Name:    dnsRecord.Domain,
		Content: dnsRecord.Address,
		Proxied: false,
	})

	return err
}

func (c *CloudflareDomainManager) DeleteDNSRecordSingle(dnsRecord *DNSRecordSingle) error {
	zoneId, err := c.getZoneIdFromDomain(dnsRecord.Domain)
	if err != nil {
		return err
	}
	return c.cfApi.DeleteDNSRecord(zoneId, dnsRecord.ProviderIdent)
}

func (c *CloudflareDomainManager) UpsertDNSRecord(dnsRecord *DNSRecord) error {
	panic("Not Implemented")
}

func (c *CloudflareDomainManager) DeleteDNSRecord(dnsRecord *DNSRecord) error {
	panic("Not Implemented")
}

func (c *CloudflareDomainManager) GetExistingDNSRecords(domain string) ([]*DNSRecord, error) {
	panic("Not Implemented")
}

func (c *CloudflareDomainManager) GetExistingDNSRecordsSingle(domain string) ([]*DNSRecordSingle, error) {
	l := c.logger.WithField("domain", domain)
	zoneId, err := c.getZoneIdFromDomain(domain)

	if err != nil {
		l.WithError(err).Warn("Failed to get zone for dns record.")
		return nil, err
	}

	// FIXME: What does Proxied:false does?
	records, err := c.cfApi.DNSRecords(zoneId, cloudflare.DNSRecord{Proxied: false})
	if err != nil {
		l.WithError(err).Warn("Failed to get dns entries.")
		return nil, err
	}

	dnsRecords := make([]*DNSRecordSingle, 0)
	for _, cloudflareRecord := range records {

		if cloudflareRecord.Name != domain || (cloudflareRecord.Type != "A" && cloudflareRecord.Type != "AAAA") {
			continue
		}

		dnsRecord := DNSRecordSingle{
			AddrType:      cloudflareRecord.Type,
			Address:       cloudflareRecord.Content,
			Domain:        cloudflareRecord.Name,
			ProviderIdent: cloudflareRecord.ID,
		}
		dnsRecords = append(dnsRecords, &dnsRecord)
	}

	return dnsRecords, nil
}

func (c *CloudflareDomainManager) CheckIfResponsible(domain string) bool {
	_, err := c.getZoneIdFromDomain(domain)
	return err == nil
}

func (c *CloudflareDomainManager) GetName() string {
	return "cloudflare"
}

func (c *CloudflareDomainManager) GetAPIType() string {
	return "single"
}

func (c *CloudflareDomainManager) UpdateCredentials(apiMail, apiKey string) error {

	if c.cfApi.APIEmail != apiMail || c.cfApi.APIKey != apiKey {
		c.logger.Info("Updating Credentials")
		cfApi, err := cloudflare.New(apiMail, apiKey)
		if err != nil {
			return err
		}

		c.cfApi = cfApi
	} else {
		c.logger.Info("Skipping Credentials Update, credentials did not change.")
	}

	return nil
}

func CreateCloudflareDomainManager(cfApiMail string, cfApiKey string) (*CloudflareDomainManager, error) {
	cfApi, err := cloudflare.New(cfApiKey, cfApiMail)
	if err != nil {
		return nil, err
	}

	cloudflareDomainHander := CloudflareDomainManager{
		zoneCache: map[string]string{},
		cfApi:     cfApi,
	}

	return &cloudflareDomainHander, nil

}
