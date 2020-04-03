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
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	"k8s.io/client-go/tools/cache"
)

// NewHandler returns an operator-sdk Handler which updates the store based on Kubernetes events
func AttachHandler(store Store, informer cache.SharedIndexInformer) {
	informer.AddEventHandler(handler{store})
}

// Implements operator-sdk.Handler; we use it to update our representation of service entries.
type handler struct {
	Store
}

func (c handler) OnAdd(obj interface{}) {
	se := obj.(*v1alpha3.ServiceEntry)
	c.Insert(se)
}

func (c handler) OnUpdate(oldObj, newObj interface{}) {
	old := oldObj.(*v1alpha3.ServiceEntry)
	se := newObj.(*v1alpha3.ServiceEntry)

	// order matters here, since it's likely these work on the same set of hosts: delete the old first, then add the new
	c.Delete(old)
	c.Insert(se)
}

func (c handler) OnDelete(obj interface{}) {
	se := obj.(*v1alpha3.ServiceEntry)
	c.Delete(se)
}
