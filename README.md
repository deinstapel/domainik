# domainik - Domain Manager Dominik

domainik is a Kubernetes operator, which automatically setup DNS records pointing to your Kubernetes cluster.

domainik currently supports the following vendors:
+ [Route53](https://aws.amazon.com/route53/) 
+ [Cloudflare](https://www.cloudflare.com/)

We defined an extensible API, so any other vendor can be implemented easily and we will be happy to merge Pull Requests.

## Installation

```sh
kubectl apply -f kubernetes/crds.yaml
kubectl apply -f kubernetes/operator.yaml
```

## Configuration

domainik is configured with CRDs.

### Domain Managers

We create a resource for every domain manager, which is backed by a secret containing the credentials.

TODO: Add Example for domain manager configuration

### Domains

We create a resource for every domain. `domainik` automagically determines which Domain Manager is responsible for this domain and uses this Domain Manager to create the correct DNS entries.

`nodeSelector` contains a standard Kubernetes node selector, which can be used to point domains to specific machines, which have an Ingress installed.

`records` contains an array of domains, which should be pointing to the matched nodes. The domains do not necessary need to be part of the same Domain Manager.

```yaml
apiVersion: dns.deinstapel.de/v1
kind: Domain
metadata:
  name: test-domain
spec:
  nodeSelector:
     node-role.kubernetes.io/worker: ""
  records:
    - "deinstapel.de"
    - "*.deinstapel.de"
```

## Versions

TODO: Release Versions