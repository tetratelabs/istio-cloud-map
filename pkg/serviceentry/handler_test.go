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
//

package serviceentry

import (
	"testing"

	"sync/atomic"

	"context"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"istio.io/istio/pilot/pkg/model"
)

func TestHandle(t *testing.T) {
	type expectedAction int
	var (
		insert expectedAction = 1
		delete expectedAction = 2
		none   expectedAction = 3
	)

	notServiceEntry, _ := crd.ConvertConfig(model.VirtualService, model.Config{
		ConfigMeta: model.ConfigMeta{
			Type:      model.VirtualService.Type,
			Group:     model.VirtualService.Group,
			Version:   model.VirtualService.Version,
			Name:      "bobby",
			Namespace: "default",
		},
		Spec: &v1alpha3.VirtualService{},
	})

	tests := []struct {
		name     string
		event    sdk.Event
		expected expectedAction
	}{
		//{"add", sdk.Event{Object: noOwners, Deleted: false}, insert},
		//{"delete", sdk.Event{Object: noOwners, Deleted: true}, delete},
		{"no-op", sdk.Event{Object: notServiceEntry, Deleted: false}, none},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeStore{}
			underTest := NewHandler(f)
			if err := underTest.Handle(context.Background(), tt.event); err != nil {
				t.Fatalf("underTest.Handle(%v) = %v wanted no err", tt.event, err)
			}

			switch tt.expected {
			case insert:
				if f.inserted != 1 || f.deleted != 0 {
					t.Fatalf("store had operations: %#v expected insert", f)
				}
			case delete:
				if f.inserted != 0 || f.deleted != 1 {
					t.Fatalf("store had operations: %#v expected delete", f)
				}
			case none:
				if f.inserted != 0 || f.deleted != 0 {
					t.Fatalf("store had operations: %#v expected none", f)
				}
			}
		})
	}
}

type fakeStore struct {
	inserted, deleted int32
	store
}

func (fakeStore) Classify(host string) Owner {
	return None
}

// Insert adds a ServiceEntry to the store (detecting who it belongs to)
func (f *fakeStore) Insert(cr crd.IstioObject) error {
	atomic.AddInt32(&f.inserted, 1)
	return nil
}

// Delete removes a ServiceEntry from the store
func (f *fakeStore) Delete(cr crd.IstioObject) error {
	atomic.AddInt32(&f.deleted, 1)
	return nil
}
