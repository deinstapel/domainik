package domainik

import metav1 "github.com/ericchiang/k8s/apis/meta/v1"

type Domain struct {
	Kind       string             `json:"kind"`
	APIVersion string             `json:"apiVersion"`
	Metadata   *metav1.ObjectMeta `json:"metadata"`
	Spec       DomainSpec         `json:"spec"`
}

func (t *Domain) GetMetadata() *metav1.ObjectMeta { return t.Metadata }

type DomainSpec struct {
	NodeSelector map[string]string `json:"nodeSelector"`
	Records []string `json:"records"`
}

type DomainManager struct {
	Kind       string             `json:"kind"`
	APIVersion string             `json:"apiVersion"`
	Metadata   *metav1.ObjectMeta `json:"metadata"`
	Spec       DomainManagerSpec         `json:"spec"`
}

func (t *DomainManager) GetMetadata() *metav1.ObjectMeta { return t.Metadata }

type DomainManagerSpec struct {
	Cloudflare *CloudflareSpec `json:"cloudflare"`
	Route53 *Route53Spec `json:"route53"`
}

type CloudflareSpec struct {
	SecretRef SecretRefSpec `json:"secretRef"`
}

type Route53Spec struct {
	SecretRef SecretRefSpec `json:"secretRef"`
}

type SecretRefSpec struct {
	Namespace string `json:"namespace"`
	Name string `json:"name"`
}

type CloudflareSecretSpec struct {
	AccessKeyId string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
}

type Route53SecretSpec struct {
	APIMail string `json:"apiMail"`
	APIKey string `json:"apiKey"`
}

