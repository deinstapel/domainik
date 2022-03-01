package domainik

import (
	"context"
	"errors"
	"github.com/deinstapel/domainik/domainmanager"
	"github.com/ericchiang/k8s"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	"github.com/sirupsen/logrus"
	"net"
	"strconv"
	"time"
)

const HostnameLabel = "kubernetes.io/hostname"

type EventHandler struct {
	// resourceName -> Domain
	domains map[string]*Domain
	// resourceName -> corev1.Node
	nodes map[string]*corev1.Node

	kubernetesClient *k8s.Client

	domainManager map[string]domainmanager.DomainManager

	logger          *logrus.Entry
	reconcileQueued bool
}

func isDeleteEvent(eventType string) bool {
	return eventType == "DELETED"
}

func isNodeDisabled(node *corev1.Node) bool {
	isDisabled := false

	for key, value := range node.Metadata.Annotations {
		if key == "dns.deinstapel.de/force-disable-node" && value == "true" {
			isDisabled = true
		}
	}

	return isDisabled
}

func isNodeReady(node *corev1.Node) bool {
	isReady := false

	for _, condition := range node.Status.Conditions {
		if *condition.Type == "Ready" && *condition.Status == "True" {
			isReady = true
		}
	}

	return isReady
}

func getIPAddresses(node *corev1.Node) ([]net.IP, []net.IP, error) {

	hostname, hasHostname := node.Metadata.Labels[HostnameLabel]
	if !hasHostname {
		return nil, nil, errors.New("node does not have a hostname")
	}

	ipList, err := net.LookupIP(hostname)

	if err != nil {
		return nil, nil, err
	}

	ipListv4 := make([]net.IP, 0)
	ipListv6 := make([]net.IP, 0)

	for _, addr := range ipList {
		isIPv4 := addr.To4() != nil

		if isIPv4 {
			ipListv4 = append(ipListv4, addr)
		} else {
			ipListv6 = append(ipListv6, addr)
		}
	}

	return ipListv4, ipListv6, nil

}

func wasNodeSignificantlyUpdated(existingNode *corev1.Node, newNode *corev1.Node) (bool, error) {

	// We probably need a lot more checks here to determine, if we should care about this node update.
	// For now, we ignore any other changes than a status change.
	// Note: ADD and DELETE events are not dropped here. This is only important for UPDATE events.

	hasReadyChange := isNodeReady(existingNode) == isNodeReady(newNode)
	return !hasReadyChange, nil
}

func (e *EventHandler) ProcessNode(eventType string, node *corev1.Node) {
	isDelete := isDeleteEvent(eventType)
	resourceName := *node.Metadata.Name
	logger := e.logger.WithField("node", resourceName)

	if isDelete {
		delete(e.nodes, resourceName)
		logger.Info("Deleted Node")
	} else {
		existingNode, isExisting := e.nodes[resourceName]

		if isExisting {
			// If we have an existing node, we need to check, if we can omit this update.

			hasSignificantChange, err := wasNodeSignificantlyUpdated(existingNode, node)

			if err != nil {
				logger.WithError(err).Warn("Could not determine if this was an important update, dropping update")
				return
			} else if !hasSignificantChange {
				logger.Info("Node was not significantly updated, dropping update")
				return
			}
		}

		// This is technically an upsert into this map.
		// We process the new map afterwards manually.
		e.nodes[resourceName] = node
		logger.Info("Updated Node")
	}

	e.MarkReconcile()
}

func (e *EventHandler) ProcessDomain(eventType string, domain *Domain) {
	isDelete := isDeleteEvent(eventType)
	resourceName := *domain.Metadata.Name
	logger := e.logger.WithField("domain", resourceName)

	if isDelete {
		delete(e.domains, resourceName)
		logger.Info("Deleted Domain")
	} else {
		e.domains[resourceName] = domain
		logger.Info("Updated Domain")
	}

	e.MarkReconcile()
}

func (e *EventHandler) GetSecret(secretRef SecretRefSpec) (map[string]string, error) {
	ctx := context.Background()
	var secret corev1.Secret
	err := e.kubernetesClient.Get(ctx, secretRef.Namespace, secretRef.Name, &secret)
	if err != nil {
		return nil, err
	}

	stringData := map[string]string{}

	for key, valByte := range secret.Data {
		val := string(valByte)
		stringData[key] = val
	}

	return stringData, nil
}

func (e *EventHandler) AddDomainManager(domainManager *DomainManager) error {
	resourceName := *domainManager.Metadata.Name
	l := e.logger.WithField("domainManager", resourceName)
	l.Info("Creating DomainManager")

	if domainManager.Spec.Cloudflare != nil {
		cloudflareSecret, err := e.GetSecret(domainManager.Spec.Cloudflare.SecretRef)

		if err != nil {
			return err
		}

		apiMail, hasApiMail := cloudflareSecret["apiMail"]
		apiKey, hasApiKey := cloudflareSecret["apiKey"]

		if !hasApiMail || !hasApiKey {
			return errors.New("invalid secret")
		}

		cloudflareDomainManager, err := domainmanager.CreateCloudflareDomainManager(apiMail, apiKey)

		if err != nil {
			return err
		}

		e.domainManager[resourceName] = cloudflareDomainManager

	} else if domainManager.Spec.Route53 != nil {
		route53Secret, err := e.GetSecret(domainManager.Spec.Route53.SecretRef)

		if err != nil {
			return err
		}

		accessKeyId, hasAccessKey := route53Secret["accessKeyId"]
		secretAccessKey, hasSecretKey := route53Secret["secretAccessKey"]

		if !hasAccessKey || !hasSecretKey {
			return errors.New("invalid secret")
		}

		route53DomainManager := domainmanager.CreateRoute53RouteManager(accessKeyId, secretAccessKey)
		e.domainManager[resourceName] = route53DomainManager

	} else {
		return errors.New("invalid DomainManager spec")
	}

	return nil

}

func (e *EventHandler) UpdateDomainManager(domainManager *DomainManager) error {
	resourceName := *domainManager.Metadata.Name
	existingDomainManager := e.domainManager[resourceName]

	l := e.logger.WithField("domainManager", resourceName)
	l.Info("Updating DomainManager")

	if domainManager.Spec.Cloudflare != nil {
		cloudflareDomainManager := existingDomainManager.(*domainmanager.CloudflareDomainManager)
		cloudflareSecret, err := e.GetSecret(domainManager.Spec.Cloudflare.SecretRef)

		if err != nil {
			return err
		}

		apiMail, hasApiMail := cloudflareSecret["apiMail"]
		apiKey, hasApiKey := cloudflareSecret["apiKey"]

		if !hasApiMail || !hasApiKey {
			return errors.New("invalid secret")
		}

		return cloudflareDomainManager.UpdateCredentials(apiMail, apiKey)
	} else if domainManager.Spec.Route53 != nil {
		route53DomainManager := existingDomainManager.(*domainmanager.Route53DomainManager)
		route53Secret, err := e.GetSecret(domainManager.Spec.Route53.SecretRef)

		if err != nil {
			return err
		}

		accessKeyId, hasAccessKey := route53Secret["accessKeyId"]
		secretAccessKey, hasSecretKey := route53Secret["secretAccessKey"]

		if !hasAccessKey || !hasSecretKey {
			return errors.New("invalid secret")
		}

		return route53DomainManager.UpdateCredentials(accessKeyId, secretAccessKey)
	} else {
		return errors.New("invalid DomainManager spec")
	}
}

func (e *EventHandler) ProcessDomainManager(eventType string, domainManager *DomainManager) {
	isDelete := isDeleteEvent(eventType)
	resourceName := *domainManager.Metadata.Name
	logger := e.logger.WithField("domainManager", resourceName)

	if isDelete {
		delete(e.domainManager, resourceName)
		logger.Info("Deleted Domain")
	} else {

		_, hasExisting := e.domainManager[resourceName]

		if !hasExisting {
			// Create new DomainManager
			err := e.AddDomainManager(domainManager)
			if err != nil {
				logger.WithError(err).Warn("Could not process domainManager, invalid configuration")
			}

		} else {
			// Maintain existing DomainManager
			err := e.UpdateDomainManager(domainManager)
			if err != nil {
				logger.WithError(err).Warn("Could not process domainManager, invalid configuration")
			}
		}
		logger.Info("Updated DomainManager")
	}

	e.MarkReconcile()
}

func (e *EventHandler) MarkReconcile() {
	e.logger.Info("Marking Resources..")
	e.reconcileQueued = true
}

func (e *EventHandler) StartReconcile(ctx context.Context) {

	timer := time.NewTicker(5 * time.Second)

	for {
		select {
		case <-timer.C:
			{
				if !e.reconcileQueued {
					continue
				}

				e.reconcileQueued = false
				e.Reconcile()
			}
		case <-ctx.Done():
			{
				timer.Stop()
				return
			}
		}
	}

}

func (e *EventHandler) Reconcile() {
	e.logger.Info("Reconcile..")

	for resourceName, domain := range e.domains {
		dLogger := e.logger.WithField("domain", resourceName)

		// Get list of nodes, which should be associated with this domain.
		nodes := make([]*corev1.Node, 0)
		for nodeResourceName, node := range e.nodes {

			if !isNodeReady(node) {
				dLogger.Warn("Omitting " + nodeResourceName + " because it is unhealthy")
				continue
			}

			if !isNodeDisabled(node) {
				dLogger.Warn("Omitting " + nodeResourceName + " because it is forcefully disabled with an annotation")
				continue
			}

			for k, v := range domain.Spec.NodeSelector {
				labelVal, labelExists := node.Metadata.Labels[k]
				if labelExists && labelVal == v {
					nodes = append(nodes, node)
					break
				}
			}
		}

		dLogger.Info("Found " + strconv.Itoa(len(nodes)) + " Nodes matching nodeSelector")

		for _, record := range domain.Spec.Records {

			rLogger := dLogger.WithField("record", record)
			var responsibleDomainManager domainmanager.DomainManager

			for _, dM := range e.domainManager {
				isResp := dM.CheckIfResponsible(record)
				if isResp {
					responsibleDomainManager = dM
					break
				}
			}

			if responsibleDomainManager == nil {
				rLogger.Warn("No responsible domain manager found, dropping")
				continue
			}

			if responsibleDomainManager.GetAPIType() == "grouped" {

				if len(nodes) > 0 {
					// Upsert

					ipv4 := make([]string, 0)
					ipv6 := make([]string, 0)
					for _, node := range nodes {
						ipv4Addresses, ipv6Addresses, err := getIPAddresses(node)

						if err != nil {
							rLogger.WithError(err).WithField("node", node.Metadata.Name).Warn("Could not get IP addresses")
							continue
						}

						for _, address := range ipv4Addresses {
							ipv4 = append(ipv4, address.String())
						}
						for _, address := range ipv6Addresses {
							ipv6 = append(ipv6, address.String())
						}
					}

					dLogger.
						WithField("len(ipv4)", len(ipv4)).
						WithField("len(ipv6)", len(ipv6)).
						Info("Processing IPs")

					upsertDNSRecordv4 := domainmanager.DNSRecord{
						AddrType:  "A",
						Addresses: ipv4,
						Domain:    record,
					}
					upsertDNSRecordv6 := domainmanager.DNSRecord{
						AddrType:  "AAAA",
						Addresses: ipv6,
						Domain:    record,
					}

					err := responsibleDomainManager.UpsertDNSRecord(&upsertDNSRecordv4)
					if err != nil {
						rLogger.WithError(err).Warn("Could not upsert DNS record for IPv4 addresses")
					}
					err = responsibleDomainManager.UpsertDNSRecord(&upsertDNSRecordv6)
					if err != nil {
						rLogger.WithError(err).Warn("Could not upsert DNS record for IPv6 addresses")
					}
				} else {
					// Delete
					existingDNSRecords, err := responsibleDomainManager.GetExistingDNSRecords(record)
					if err != nil {
						rLogger.WithError(err).Warn("Could not get existing DNS records")
					}

					for _, dnsRecord := range existingDNSRecords {
						err = responsibleDomainManager.DeleteDNSRecord(dnsRecord)
						if err != nil {
							rLogger.WithError(err).Warn("Could not delete existing DNS records")
						}
					}
				}

				dLogger.Info("Configured Domain")

			} else {
				// apiType == "single"
				existingDNSRecords, err := responsibleDomainManager.GetExistingDNSRecordsSingle(record)
				if err != nil {
					rLogger.WithError(err).Warn("Could not get DNS records")
				}

				desiredDNSRecords := make([]*domainmanager.DNSRecordSingle, 0)
				for _, node := range nodes {
					ipv4Addresses, ipv6Addresses, err := getIPAddresses(node)

					if err != nil {
						rLogger.WithError(err).WithField("node", node.Metadata.Name).Warn("Could not get IP addresses")
						continue
					}

					for _, address := range ipv4Addresses {
						desiredDNSRecords = append(
							desiredDNSRecords,
							&domainmanager.DNSRecordSingle{
								AddrType: "A",
								Address:  address.String(),
								Domain:   record,
							},
						)
					}
					for _, address := range ipv6Addresses {
						desiredDNSRecords = append(
							desiredDNSRecords,
							&domainmanager.DNSRecordSingle{
								AddrType: "AAAA",
								Address:  address.String(),
								Domain:   record,
							},
						)
					}
				}

				recordsToCreate := make([]*domainmanager.DNSRecordSingle, 0)
				recordsToRemove := make([]*domainmanager.DNSRecordSingle, 0)

				for _, existingDNSRecord := range existingDNSRecords {
					isDesired := false
					for _, desiredDNSRecord := range desiredDNSRecords {
						if existingDNSRecord.Domain == desiredDNSRecord.Domain &&
							existingDNSRecord.AddrType == desiredDNSRecord.AddrType &&
							existingDNSRecord.Address == desiredDNSRecord.Address {
							isDesired = true
						}
					}

					if !isDesired {
						recordsToRemove = append(recordsToRemove, existingDNSRecord)
					}
				}

				for _, desiredDNSRecord := range desiredDNSRecords {
					isExisting := false
					for _, existingDNSRecord := range existingDNSRecords {
						if desiredDNSRecord.Domain == existingDNSRecord.Domain &&
							desiredDNSRecord.AddrType == existingDNSRecord.AddrType &&
							desiredDNSRecord.Address == existingDNSRecord.Address {
							isExisting = true
						}
					}

					if !isExisting {
						recordsToCreate = append(recordsToCreate, desiredDNSRecord)
					}
				}

				dLogger.
					WithField("recordsToCreate", len(recordsToCreate)).
					WithField("recordsToRemove", len(recordsToRemove)).
					Info("Calculated Diff")

				for _, recordToCreate := range recordsToCreate {
					err = responsibleDomainManager.CreateDNSRecordSingle(recordToCreate)
					if err != nil {
						rLogger.WithError(err).
							WithField("address", recordToCreate.Address).
							WithField("type", recordToCreate.AddrType).
							Warn("Could not create DNS record")
					}
				}

				for _, recordToRemove := range recordsToRemove {
					err = responsibleDomainManager.DeleteDNSRecordSingle(recordToRemove)
					if err != nil {
						rLogger.WithError(err).
							WithField("address", recordToRemove.Address).
							WithField("type", recordToRemove.AddrType).
							Warn("Could not delete DNS record")
					}
				}

				dLogger.Info("Configured Domain")
			}
		}
	}
}

func CreateEventHandler(logger *logrus.Entry, client *k8s.Client) *EventHandler {

	watchComb := EventHandler{
		domains:          map[string]*Domain{},
		nodes:            map[string]*corev1.Node{},
		domainManager:    map[string]domainmanager.DomainManager{},
		kubernetesClient: client,
		logger:           logger.WithField("module", "watch-combinator"),
		reconcileQueued:  false,
	}

	return &watchComb
}
