package domainik

import (
	"context"
	"errors"
	"github.com/deinstapel/domainik/domainmanager"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	"github.com/sirupsen/logrus"
	"net"
	"strconv"
	"time"
)

const HostnameLabel = "kubernetes.io/hostname"

type WatchCombinator struct {
	// resourceName -> Domain
	domains map[string]*Domain
	// resourceName -> corev1.Node
	nodes map[string]*corev1.Node

	domainManager []domainmanager.DomainManager

	logger          *logrus.Entry
	reconcileQueued bool
}

func isDeleteEvent(eventType string) bool {
	return eventType == "DELETED"
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

func (w *WatchCombinator) ProcessNode(eventType string, node *corev1.Node) {
	isDelete := isDeleteEvent(eventType)
	resourceName := *node.Metadata.Name
	logger := w.logger.WithField("node", resourceName)

	if isDelete {
		delete(w.nodes, resourceName)
		logger.Info("Deleted Node")
	} else {
		existingNode, isExisting := w.nodes[resourceName]

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
		w.nodes[resourceName] = node
		logger.Info("Updated Node")
	}

	w.MarkReconcile()
}

func (w *WatchCombinator) ProcessDomain(eventType string, domain *Domain) {
	isDelete := isDeleteEvent(eventType)
	resourceName := *domain.Metadata.Name
	logger := w.logger.WithField("domain", resourceName)

	if isDelete {
		delete(w.domains, resourceName)
		logger.Info("Deleted Domain")
	} else {
		w.domains[resourceName] = domain
		logger.Info("Updated Domain")
	}

	w.MarkReconcile()
}

func (w *WatchCombinator) MarkReconcile() {
	w.logger.Info("Marking Resources..")
	w.reconcileQueued = true
}

func (w *WatchCombinator) StartReconcile(ctx context.Context) {

	timer := time.NewTicker(5 * time.Second)

	for {
		select {
		case <-timer.C:
			{
				if !w.reconcileQueued {
					continue
				}

				w.reconcileQueued = false
				w.Reconcile()
			}
		case <-ctx.Done():
			{
				timer.Stop()
				return
			}
		}
	}

}

func (w *WatchCombinator) Reconcile() {
	w.logger.Info("Reconcile..")

	for resourceName, domain := range w.domains {
		dLogger := w.logger.WithField("domain", resourceName)

		// Get list of nodes, which should be associated with this domain.
		nodes := make([]*corev1.Node, 0)
		for nodeResourceName, node := range w.nodes {

			if !isNodeReady(node) {
				dLogger.Warn("Omitting " + nodeResourceName + " because it is unhealthy")
				continue
			}

			for k, v := range domain.Spec.NodeSelector {
				labelVal, labelExists := node.Metadata.Labels[k]
				if  labelExists && labelVal == v {
					nodes = append(nodes, node)
					break
				}
			}
		}

		dLogger.Info("Found " + strconv.Itoa(len(nodes)) + " Nodes matching nodeSelector")

		for _, record := range domain.Spec.Records {

			rLogger := dLogger.WithField("record", record)
			var responsibleDomainManager domainmanager.DomainManager

			for _, dM := range w.domainManager {
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



			} else {
				// apiType == "single"
			}

		}
	}


}

func CreateWatchCombinator(logger *logrus.Entry, domainManager []domainmanager.DomainManager) *WatchCombinator {

	watchComb := WatchCombinator{
		domains:         map[string]*Domain{},
		nodes:           map[string]*corev1.Node{},
		domainManager:   domainManager,
		logger:          logger.WithField("module", "watch-combinator"),
		reconcileQueued: false,
	}

	return &watchComb
}
