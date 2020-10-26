package domainmanager

import (
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/sirupsen/logrus"
	"strings"
)

type Route53DomainManager struct {
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

func (d *Route53DomainManager) GetHostedZoneForDomain(domain string) (*route53.HostedZone, error){
	domainParts := strings.Split(domain, ".")
	dnsName := fmt.Sprintf("%s.%s.", domainParts[len(domainParts)-2], domainParts[len(domainParts)-1])
	if cachedHostedZone, ok := d.hostedZoneCache[dnsName]; ok {
		return cachedHostedZone, nil
	}

	hostedZones, err := d.route53Api.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{DNSName: &dnsName})

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
	d.hostedZoneCache[dnsName] = hostedZone
	d.logger.WithField("name", *hostedZone.Name).Info("Cached HostedZone")

	return hostedZone, nil
}

func (d *Route53DomainManager) CheckIfResponsible(record string) bool {
	_, err := d.GetHostedZoneForDomain(record)
	return err == nil
}


func (d *Route53DomainManager) UpsertDNSRecord(dnsRecord *DNSRecord) error {
	hostedZone, err := d.GetHostedZoneForDomain(dnsRecord.Domain)
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

	_, err = d.route53Api.ChangeResourceRecordSets(batchRequest)

	return err

}
func (d *Route53DomainManager) DeleteDNSRecord(dnsRecord *DNSRecord) error {
	hostedZone, err := d.GetHostedZoneForDomain(dnsRecord.Domain);

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

	_, err = d.route53Api.ChangeResourceRecordSets(batchRequest)

	return err
}

func (d *Route53DomainManager) CreateDNSRecordSingle(dnsRecord *DNSRecordSingle) error {
	panic("Not Implemented")
}
func (d *Route53DomainManager) DeleteDNSRecordSingle(dnsRecord *DNSRecordSingle) error {
	panic("Not Implemented")
}

func (d *Route53DomainManager) GetExistingDNSRecords(domain string) ([]*DNSRecord, error) {
	hostedZone, err := d.GetHostedZoneForDomain(domain)

	if err != nil {
		return nil, err
	}

	resourceRecords, _ := d.route53Api.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
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
func (d *Route53DomainManager) GetExistingDNSRecordsSingle(domain string) ([]*DNSRecordSingle, error) {
	panic("Not Implemented")
}

func (d *Route53DomainManager) GetAPIType() string{
	return "grouped"
}


func (d *Route53DomainManager) GetName() string{
	return "route53"
}

func getRoute53ClientFromCredentials(awsAccessKey, secretAccessKey string) *route53.Route53 {
	awsCredentials := credentials.NewStaticCredentials(awsAccessKey, secretAccessKey, "")
	mySession := session.Must(session.NewSession())
	route53Api := route53.New(mySession, aws.NewConfig().WithCredentials(awsCredentials).WithRegion("eu-central-1"))
	return route53Api
}

func (d *Route53DomainManager) UpdateCredentials(accessKeyId, secretAccessKey string) error {

	currentCredentials, err := d.route53Api.Config.Credentials.Get()
	if err != nil {
		return err
	}

	if currentCredentials.AccessKeyID != accessKeyId || currentCredentials.SecretAccessKey != secretAccessKey {
		d.logger.Info("Updating Credentials")
		d.route53Api = getRoute53ClientFromCredentials(accessKeyId, secretAccessKey)
	} else {
		d.logger.Info("Skipping Credentials Update, credentials did not change.")
	}

	return nil
}

func CreateRoute53RouteManager(awsAccessKey string, secretAccessKey string) *Route53DomainManager {

	route53Api := getRoute53ClientFromCredentials(awsAccessKey, secretAccessKey)
	route53DomainHandler := Route53DomainManager{
		route53Api: route53Api,
		hostedZoneCache: map[string]*route53.HostedZone{},
		logger: logrus.WithField("domain-handler", "route53"),
	}

	return &route53DomainHandler
}