package domainmanager

import (
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
)

type Route53DomainHandler struct {
	route53Api *route53.Route53
	hostedZoneCache map[string]*route53.HostedZone
	logger *logrus.Entry
}

func unescapeRoute53URL(s string) string {

	retS := s

	if strings.HasSuffix(s, ".") {
		retS = strings.TrimSuffix(s, ".")
	}

	return strings.ReplaceAll(retS, "\\052", "*")


}

func (domainHandler *Route53DomainHandler) GetHostedZoneForDomain(domain string) (*route53.HostedZone, error){
	domainParts := strings.Split(domain, ".")
	dnsName := fmt.Sprintf("%s.%s.", domainParts[len(domainParts)-2], domainParts[len(domainParts)-1])
	if cachedHostedZone, ok := domainHandler.hostedZoneCache[dnsName]; ok {
		return cachedHostedZone, nil
	}

	hostedZones, err := domainHandler.route53Api.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{DNSName: &dnsName})

	if err != nil {
		return nil, err
	}

	if len(hostedZones.HostedZones) < 1 {
		return nil, errors.New("HostedZones was empty")
	}

	hostedZone := hostedZones.HostedZones[0]
	if *hostedZone.Name != dnsName {
		return nil, errors.New("Invalid Reply from Route53")
	}
	domainHandler.hostedZoneCache[dnsName] = hostedZone
	fmt.Fprintf(os.Stderr, "[Route53] Cached HostedZone '%v'\n", *hostedZone.Name)

	return hostedZone, nil
}

func (domainHandler *Route53DomainHandler) CheckIfResponsible(record string) bool {
	_, err := domainHandler.GetHostedZoneForDomain(record)
	return err == nil
}


func (domainHandler *Route53DomainHandler) UpsertDNSRecord(dnsRecord *DNSRecord) error {
	hostedZone, err := domainHandler.GetHostedZoneForDomain(dnsRecord.Domain)
	if err != nil {
		return err
	}

	resourceRecords := make([]*route53.ResourceRecord, 0)

	for _, ipAddr := range dnsRecord.Addresses {
		resourceRecords = append(
			resourceRecords,
			&route53.ResourceRecord{
				Value: aws.String(ipAddr),
			},
		)
	}

	batchRequest := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("UPSERT"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						ResourceRecords: resourceRecords,
						Name: &dnsRecord.Domain,
						Type: &dnsRecord.AddrType,
						TTL: aws.Int64(60),
					},
				},
			},
			Comment: aws.String("Managed by dns-bot"),
		},
		HostedZoneId: hostedZone.Id,
	}

	_, err = domainHandler.route53Api.ChangeResourceRecordSets(batchRequest)

	return err

}
func (domainHandler *Route53DomainHandler) DeleteDNSRecord(dnsRecord *DNSRecord) error {
	hostedZone, err := domainHandler.GetHostedZoneForDomain(dnsRecord.Domain);

	if err != nil {
		return err
	}

	resourceRecords := make([]*route53.ResourceRecord, 0)

	for _, ipAddr := range dnsRecord.Addresses {
		resourceRecords = append(
			resourceRecords,
			&route53.ResourceRecord{
				Value: aws.String(ipAddr),
			},
		)
	}

	batchRequest := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("DELETE"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						ResourceRecords: resourceRecords,
						Name: &dnsRecord.Domain,
						Type: &dnsRecord.AddrType,
						TTL: aws.Int64(60),
					},
				},
			},
		},
		HostedZoneId: hostedZone.Id,
	}

	_, err = domainHandler.route53Api.ChangeResourceRecordSets(batchRequest)

	return err
}

func (_ *Route53DomainHandler) CreateDNSRecordSingle(dnsRecord *DNSRecordSingle) error {
	panic("Not Implemented")
}
func (_ *Route53DomainHandler) DeleteDNSRecordSingle(dnsRecord *DNSRecordSingle) error {
	panic("Not Implemented")
}

func (domainHandler *Route53DomainHandler) GetExistingDNSRecords(domain string) ([]*DNSRecord, error) {
	hostedZone, err := domainHandler.GetHostedZoneForDomain(domain)

	if err != nil {
		return nil, err
	}

	resourceRecords, _ := domainHandler.route53Api.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId: hostedZone.Id,
	})

	dnsRecords := make([]*DNSRecord, 0)


	for _, resourceRecord := range resourceRecords.ResourceRecordSets {

		if *resourceRecord.Type != "A" && *resourceRecord.Type != "AAAA" {
			continue
		}

		escapedDomain := unescapeRoute53URL(*resourceRecord.Name)
		isSameDomain := escapedDomain == domain

		// We only need A and AAAA records. We do not manage other records.
		if !isSameDomain {
			continue
		}

		addresses := make([]string, 0)

		for _, record := range resourceRecord.ResourceRecords {
			addresses = append(addresses, *record.Value)
		}

		dnsRecords = append(dnsRecords, &DNSRecord{
			Domain:  domain,
			AddrType: *resourceRecord.Type,
			Addresses: addresses,
		})

	}

	return dnsRecords, nil
}
func (_ *Route53DomainHandler) GetExistingDNSRecordsSingle(domain string) ([]*DNSRecordSingle, error) {
	panic("Not Implemented")
}

func (_ *Route53DomainHandler) GetAPIType() string{
	return "grouped"
}


func (_ *Route53DomainHandler) GetName() string{
	return "route53"
}


func CreateRoute53RouteHandler(awsAccessKey string, awsSecretKey string) *Route53DomainHandler {

	awsCredentials := credentials.NewStaticCredentials(awsAccessKey, awsSecretKey, "")
	mySession := session.Must(session.NewSession())
	route53Api := route53.New(mySession, aws.NewConfig().WithCredentials(awsCredentials).WithRegion("eu-central-1"))

	route53DomainHandler := Route53DomainHandler{
		route53Api: route53Api,
		hostedZoneCache: map[string]*route53.HostedZone{},
		logger: logrus.WithField("domain-handler", "route53"),
	}

	return &route53DomainHandler
}