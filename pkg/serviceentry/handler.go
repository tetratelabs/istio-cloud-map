package serviceentry

import (
	"context"
	"reflect"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var serviceEntryGroupVersionKind = schema.GroupVersionKind{
	Group:   "networking.istio.io",
	Version: "v1alpha3",
	Kind:    "ServiceEntry",
}

// NewHandler returns an operator-sdk Handler which updates the store based on Kubernetes events
func NewHandler(store Store) sdk.Handler {
	return handler{store}
}

// Implements operator-sdk.Handler; we use it to update our representation of service entries.
type handler struct {
	Store
}

func (h handler) Handle(ctx context.Context, event sdk.Event) error {
	switch cr := event.Object.(type) {
	case crd.IstioObject:
		if !reflect.DeepEqual(cr.GetObjectKind().GroupVersionKind(), serviceEntryGroupVersionKind) {
			return nil
		}
		if event.Deleted {
			return h.Delete(cr)
		}
		return h.Insert(cr)
	}
	return nil
}
