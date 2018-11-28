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
	"fmt"
	"testing"

	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"istio.io/istio/pilot/pkg/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	id = "123"
	t  = true

	baseOwner = v1.OwnerReference{
		APIVersion: "route53.istio.io",
		Kind:       "ServiceController",
		Name:       id,
		Controller: &t,
	}

	noOwners = createIstioObject(
		&v1alpha3.ServiceEntry{
			Hosts: []string{"no.owners"},
		},
	)

	us = createIstioObject(
		&v1alpha3.ServiceEntry{
			Hosts: []string{"1.us", "2.us"},
		},
		baseOwner,
	)

	them = createIstioObject(
		&v1alpha3.ServiceEntry{
			Hosts: []string{"1.them", "2.them", "3.them"},
		},
		v1.OwnerReference{
			APIVersion: "route53.istio.io",
			Kind:       "ServiceController",
			Name:       "789",
			Controller: &t,
		},
	)
)

func TestInsert(t *testing.T) {
	tests := []struct {
		name         string
		crs          []crd.IstioObject
		ours, theirs []string
	}{
		{
			"empty",
			[]crd.IstioObject{},
			[]string{},
			[]string{},
		},
		{
			"no owners",
			[]crd.IstioObject{noOwners},
			[]string{"no.owners"},
			[]string{},
		},
		{
			"us",
			[]crd.IstioObject{us},
			[]string{"1.us", "2.us"},
			[]string{},
		},
		{
			"them",
			[]crd.IstioObject{them},
			[]string{},
			[]string{"1.them", "2.them", "3.them"},
		},
		{
			"no owners, us",
			[]crd.IstioObject{noOwners, us},
			[]string{"no.owners", "1.us", "2.us"},
			[]string{},
		},
		{
			"no owners, us, them",
			[]crd.IstioObject{noOwners, us, them},
			[]string{"no.owners", "1.us", "2.us"},
			[]string{"1.them", "2.them", "3.them"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			underTest := NewLoggingStore(New(baseOwner), t.Logf)
			for _, o := range tt.crs {
				if err := underTest.Insert(o); err != nil {
					t.Fatalf("New(%q).Insert(%v) = %v wanted no err", id, o, err)
				}
			}

			ours := underTest.Ours()
			if len(ours) != len(tt.ours) {
				t.Errorf("len(underTest.Ours()) = %d expected %d", len(ours), len(tt.ours))
			}
			for _, target := range tt.ours {
				if _, exists := ours[target]; !exists {
					t.Errorf("expected host %q in ours, but not found: %v", target, ours)
				}
			}

			theirs := underTest.Theirs()
			if len(theirs) != len(tt.theirs) {
				t.Errorf("len(underTest.Theirs()) = %d expected %d", len(theirs), len(tt.theirs))
			}
			for _, target := range tt.theirs {
				if _, exists := theirs[target]; !exists {
					t.Errorf("expected host %q in ours, but not found: %v", target, theirs)
				}
			}
		})
	}
}

func TestDelete(t *testing.T) {
	// in this test we add all IstioObjects in the test, then delete out crs. Ours, Theirs should be
	// the remaining hostnames after the deletion.
	tests := []struct {
		name         string
		crs          []crd.IstioObject
		ours, theirs []string
	}{
		{
			"empty",
			[]crd.IstioObject{},
			[]string{"no.owners", "1.us", "2.us"},
			[]string{"1.them", "2.them", "3.them"},
		},
		{
			"no owners",
			[]crd.IstioObject{noOwners},
			[]string{"1.us", "2.us"},
			[]string{"1.them", "2.them", "3.them"},
		},
		{
			"us",
			[]crd.IstioObject{us},
			[]string{"no.owners"},
			[]string{"1.them", "2.them", "3.them"},
		},
		{
			"them",
			[]crd.IstioObject{them},
			[]string{"no.owners", "1.us", "2.us"},
			[]string{},
		},
		{
			"no owners, us",
			[]crd.IstioObject{noOwners, us},
			[]string{},
			[]string{"1.them", "2.them", "3.them"},
		},
		{
			"no owners, us, them",
			[]crd.IstioObject{noOwners, us, them},
			[]string{},
			[]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			underTest := NewLoggingStore(New(baseOwner), t.Logf)
			for _, o := range []crd.IstioObject{noOwners, us, them} {
				if err := underTest.Insert(o); err != nil {
					t.Fatalf("New(%q).Insert(%v) = %v wanted no err", id, o, err)
				}
			}
			for _, d := range tt.crs {
				if err := underTest.Delete(d); err != nil {
					t.Fatalf("New(%q).Delete(%v) = %v wanted no err", id, d, err)
				}
			}

			ours := underTest.Ours()
			if len(ours) != len(tt.ours) {
				t.Errorf("len(underTest.Ours()) = %d expected %d; %v", len(ours), len(tt.ours), ours)
			}
			for _, target := range tt.ours {
				if _, exists := ours[target]; !exists {
					t.Errorf("expected host %q in ours, but not found: %v", target, ours)
				}
			}

			theirs := underTest.Theirs()
			if len(theirs) != len(tt.theirs) {
				t.Errorf("len(underTest.Theirs()) = %d expected %d", len(theirs), len(tt.theirs))
			}
			for _, target := range tt.theirs {
				if _, exists := theirs[target]; !exists {
					t.Errorf("expected host %q in ours, but not found: %v", target, theirs)
				}
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name         string
		crs          []crd.IstioObject
		ours, theirs []string
	}{
		{
			"empty",
			[]crd.IstioObject{},
			[]string{},
			[]string{},
		},
		{
			"no owners",
			[]crd.IstioObject{noOwners},
			[]string{"no.owners"},
			[]string{},
		},
		{
			"us",
			[]crd.IstioObject{us},
			[]string{"1.us", "2.us"},
			[]string{},
		},
		{
			"them",
			[]crd.IstioObject{them},
			[]string{},
			[]string{"1.them", "2.them", "3.them"},
		},
		{
			"no owners, us",
			[]crd.IstioObject{noOwners, us},
			[]string{"no.owners", "1.us", "2.us"},
			[]string{},
		},
		{
			"no owners, us, them",
			[]crd.IstioObject{noOwners, us, them},
			[]string{"no.owners", "1.us", "2.us"},
			[]string{"1.them", "2.them", "3.them"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			underTest := NewLoggingStore(New(baseOwner), t.Logf)
			for _, o := range tt.crs {
				if err := underTest.Insert(o); err != nil {
					t.Fatalf("New(%q).Insert(%v) = %v wanted no err", id, o, err)
				}
			}

			for _, o := range tt.ours {
				if actual := underTest.Classify(o); actual != Us {
					t.Errorf("underTest.Classify(%q) = %d", o, actual)
				}
			}

			for _, o := range tt.theirs {
				if actual := underTest.Classify(o); actual != Them {
					t.Errorf("underTest.Classify(%q) = %d", o, actual)
				}
			}
		})
	}
}

func createIstioObject(se *v1alpha3.ServiceEntry, owners ...v1.OwnerReference) crd.IstioObject {
	o, err := crd.ConvertConfig(model.ServiceEntry, model.Config{
		ConfigMeta: model.ConfigMeta{
			Type:      model.ServiceEntry.Type,
			Group:     model.ServiceEntry.Group,
			Version:   model.ServiceEntry.Version,
			Name:      "bobby",
			Namespace: "default",
		},
		Spec: se,
	})
	if err != nil {
		panic(fmt.Errorf("crd.ConvertConfig failed with %v", err))
	}
	m := o.GetObjectMeta()
	m.OwnerReferences = owners
	o.SetObjectMeta(m)
	return o
}
