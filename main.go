package main

import (
	"context"
	"github.com/deinstapel/domainik/domainik"
	"github.com/deinstapel/domainik/domainmanager"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ericchiang/k8s"
	"github.com/ghodss/yaml"
)

func init() {
	k8s.Register("dns.deinstapel.de", "v1", "domains", false, &domainik.Domain{})
}

func watchChanges(
	ctx context.Context,
	kubernetesClient *k8s.Client,
	resourceFactory func() k8s.Resource,
	processEntry func(eventType string, resource k8s.Resource),
) error {

	// Watch configmaps in the "kube-system" namespace

	for {
		node := resourceFactory()
		watcher, err := kubernetesClient.Watch(ctx, k8s.AllNamespaces, node)
		if err != nil {
			return err
		}

		for {
			entity := resourceFactory()
			eventType, err := watcher.Next(entity)

			if err != nil && err == context.Canceled {
				watcher.Close()
				return nil
			} else if err != nil {
				watcher.Close()
				break
			}

			processEntry(eventType, entity)
		}
	}
}

func watchNodeChanges(ctx context.Context, kubernetesClient *k8s.Client, watchCombinator *domainik.WatchCombinator) error {

	return watchChanges(
		ctx,
		kubernetesClient,
		func() k8s.Resource {
			var node corev1.Node
			return &node
		},
		func(eventType string, resource k8s.Resource) {
			node := resource.(*corev1.Node)
			watchCombinator.ProcessNode(eventType, node)
		},
	)

}

func watchDomainChanges(ctx context.Context, kubernetesClient *k8s.Client, watchCombinator *domainik.WatchCombinator) error {

	return watchChanges(
		ctx,
		kubernetesClient,
		func() k8s.Resource {
			var domain domainik.Domain
			return &domain
		},
		func(eventType string, resource k8s.Resource) {
			domain := resource.(*domainik.Domain)
			watchCombinator.ProcessDomain(eventType, domain)
		},
	)
}

func main() {

	logrus.SetFormatter(&logrus.TextFormatter{})
	logger := logrus.WithField("app", "domainik")

	client, err := makeClient()
	if err != nil {
		logger.WithError(err).Error("Failed to create kubernetes client")
		os.Exit(1)
	}

	domainManagers := make([]domainmanager.DomainManager, 0)

	route53DomainManager := domainmanager.CreateRoute53RouteHandler("", "")
	domainManagers = append(domainManagers, route53DomainManager)

	watchCombinator := domainik.CreateWatchCombinator(logger, domainManagers)

	watchContext := context.Background()
	watchContext, cancelWatchContext := context.WithCancel(watchContext)
	reconcileContext := context.Background()
	reconcileContext, cancelReconcileContext := context.WithCancel(watchContext)

	stopApplication := func() {
		cancelWatchContext()
		cancelReconcileContext()
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		err := watchDomainChanges(watchContext, client, watchCombinator)
		if err != nil {
			logger.WithError(err).Error("Failed to watch Domain changes")
			stopApplication()
		}

		wg.Done()
	}()

	wg.Add(1)
	go func() {
		err := watchNodeChanges(watchContext, client, watchCombinator)
		if err != nil {
			logger.WithError(err).Error("Failed to watch Node changes")
			stopApplication()
		}

		wg.Done()
	}()

	wg.Add(1)
	go func() {
		watchCombinator.StartReconcile(reconcileContext)
		wg.Done()
	}()

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.WithField("signal", sig).Info("Received Signal")

	stopApplication()
	wg.Wait()
}

func makeKubeconfigClient(path string) (*k8s.Client, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config := new(k8s.Config)
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}
	client, err := k8s.NewClient(config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func makeClient() (*k8s.Client, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return makeKubeconfigClient(kubeconfig)
	}
	return k8s.NewInClusterClient()
}
