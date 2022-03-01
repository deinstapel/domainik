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

domainik is configured with CRDs and Annotations.

### Domain Managers

We create a resource for every domain manager, which is backed by a secret containing the credentials.

#### Route53

```yaml
apiVersion: dns.deinstapel.de/v1
kind: DomainManager
metadata:
  name: cloudflare
spec:
  route53: 
    secretRef:
      name: route53
      namespace: domains
```

Therefore you need a fitting secret with the keys `apiMail`, which contains your account mail address, 
and `apiKey`, which contains your API key.


#### Cloudflare

```yaml
apiVersion: dns.deinstapel.de/v1
kind: DomainManager
metadata:
  name: cloudflare
spec:
  cloudflare:
    secretRef:
      name: cloudflare
      namespace: domains
```
Therefore you need a fitting secret with the keys `accessKeyId`, which contains your access key id, 
and `secretAccessKey`, which contains the corresponding secret.


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

### Annotations

We have some annotations for convenience functions.

#### Disabling a node forcefully.

`dns.deinstapel.de/force-disable-node: true` disables a node forcefully, regardless of their node state.

## Versions

TODO: Release Versions