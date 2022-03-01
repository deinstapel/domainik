package domainmanager

import (
	"github.com/sirupsen/logrus"
	"os"
)

type DomainManagerDryRun struct {
	isDryRun      bool
	domainManager DomainManager
	logger        *logrus.Entry
}

func (dM *DomainManagerDryRun) CreateDNSRecordSingle(dnsRecord *DNSRecordSingle) error {
	if dM.isDryRun {
		dM.logger.Info("Skipping CreateDNSRecordSingle for dnsRecord=%v, because isDryRun is true", dnsRecord.Domain)
		return nil
	}
	return dM.domainManager.CreateDNSRecordSingle(dnsRecord)
}
func (dM *DomainManagerDryRun) DeleteDNSRecordSingle(dnsRecord *DNSRecordSingle) error {
	if dM.isDryRun {
		dM.logger.Info("Skipping DeleteDNSRecordSingle for dnsRecord=%v, because isDryRun is true", dnsRecord.Domain)
		return nil
	}
	return dM.domainManager.DeleteDNSRecordSingle(dnsRecord)
}
func (dM *DomainManagerDryRun) UpsertDNSRecord(dnsRecord *DNSRecord) error {
	if dM.isDryRun {
		dM.logger.Info("Skipping UpsertDNSRecord for dnsRecord=%v, because isDryRun is true", dnsRecord.Domain)
		return nil
	}
	return dM.domainManager.UpsertDNSRecord(dnsRecord)
}
func (dM *DomainManagerDryRun) DeleteDNSRecord(dnsRecord *DNSRecord) error {
	if dM.isDryRun {
		dM.logger.Info("Skipping DeleteDNSRecord for dnsRecord=%v, because isDryRun is true", dnsRecord.Domain)
		return nil
	}
	return dM.domainManager.DeleteDNSRecord(dnsRecord)
}
func (dM *DomainManagerDryRun) GetExistingDNSRecords(domain string) ([]*DNSRecord, error) {
	return dM.domainManager.GetExistingDNSRecords(domain)
}
func (dM *DomainManagerDryRun) GetExistingDNSRecordsSingle(domain string) ([]*DNSRecordSingle, error) {
	return dM.domainManager.GetExistingDNSRecordsSingle(domain)
}
func (dM *DomainManagerDryRun) CheckIfResponsible(domain string) bool {
	return dM.domainManager.CheckIfResponsible(domain)
}
func (dM *DomainManagerDryRun) GetName() string {
	return dM.domainManager.GetName()
}
func (dM *DomainManagerDryRun) GetAPIType() string {
	return dM.domainManager.GetAPIType()
}

func WrapIntoDryRunProtector(domainManager DomainManager) *DomainManagerDryRun {
	logger := logrus.WithField("domain-handler", "dry-run-protect")

	dryRun := false
	dryRunValue, ok := os.LookupEnv("DOMAINIK_DRY_RUN")
	if ok && dryRunValue == "true" {
		dryRun = true
	}

	return &DomainManagerDryRun{
		isDryRun:      dryRun,
		domainManager: domainManager,
		logger:        logger,
	}
}
