package serviceentry

import (
	"istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Entry struct {
	Meta v1.ObjectMeta
	Spec *v1alpha3.ServiceEntry
}

func (e *Entry) Endpoints() []*v1alpha3.ServiceEntry_Endpoint {
	return e.Spec.GetEndpoints()
}
