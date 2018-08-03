// Copyright 2018 Tetrate Labs
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
