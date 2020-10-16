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
