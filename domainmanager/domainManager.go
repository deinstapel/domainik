package domainmanager

type DNSRecord struct {
	AddrType  string
	Addresses []string
	Domain    string
}

type DNSRecordSingle struct {
	AddrType string
	Address string
	Domain string
	ProviderIdent string
}

type DomainManager interface {
	CreateDNSRecordSingle(dnsRecord *DNSRecordSingle) error
	DeleteDNSRecordSingle(dnsRecord *DNSRecordSingle) error
	UpsertDNSRecord(dnsRecord *DNSRecord) error
	DeleteDNSRecord(dnsRecord *DNSRecord) error
	GetExistingDNSRecords(domain string) ([]*DNSRecord, error)
	GetExistingDNSRecordsSingle(domain string) ([]*DNSRecordSingle, error)
	CheckIfResponsible(domain string) bool
	GetName() string
	GetAPIType() string
}

