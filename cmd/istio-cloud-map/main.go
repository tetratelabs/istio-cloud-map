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

package main

import (
	"context"
	"log"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/tetratelabs/istio-cloud-map/pkg/serviceentry"
	ic "istio.io/client-go/pkg/clientset/versioned"
	icinformer "istio.io/client-go/pkg/informers/externalversions/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/tetratelabs/istio-cloud-map/pkg/cloudmap"
	"github.com/tetratelabs/istio-cloud-map/pkg/control"
)

const (
	apiGroup      = "networking.istio.io"
	apiVersion    = "v1alpha3"
	apiType       = apiGroup + "/" + apiVersion
	kind          = "ServiceEntry"
	allNamespaces = ""
	resyncPeriod  = 30
)

func serve() (serve *cobra.Command) {
	var (
		id         string
		debug      bool
		kubeConfig string
		namespace  string
		awsRegion  string
	)

	serve = &cobra.Command{
		Use:     "serve",
		Aliases: []string{"serve"},
		Short:   "Starts the Istio Cloud Map Operator server",
		Example: "istio-cloud-map serve --id 123",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
			if err != nil {
				return errors.Wrapf(err, "failed to create a kube client from the config %q", kubeConfig)
			}
			ic, err := ic.NewForConfig(cfg)
			if err != nil {
				return errors.Wrap(err, "failed to create an istio client from the k8s rest config")
			}

			t := true
			owner := v1.OwnerReference{
				APIVersion: "cloudmap.istio.io",
				Kind:       "ServiceController",
				Name:       id,
				Controller: &t,
			}

			// TODO: move over to run groups, get a context there to use to handle shutdown gracefully.
			ctx := context.Background() // common context for cancellation across all loops/routines

			cloudMap := cloudmap.NewStore()
			log.Printf("Starting Cloud Map watcher in %q", awsRegion)
			cmWatcher, err := cloudmap.NewWatcher(cloudMap, awsRegion)
			if err != nil {
				return err
			}
			go cmWatcher.Run(ctx)

			istio := serviceentry.New(owner)
			if debug {
				istio = serviceentry.NewLoggingStore(istio, log.Printf)
			}
			log.Print("Starting Synchronizer control loop")
			sync := control.NewSynchronizer(owner, istio, cloudMap, ic.NetworkingV1alpha3().ServiceEntries(allNamespaces))
			go sync.Run(ctx)

			informer := icinformer.NewServiceEntryInformer(ic, allNamespaces, 5*time.Second,
				// taken from https://github.com/istio/istio/blob/release-1.5/pilot/pkg/bootstrap/namespacecontroller.go
				cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
			serviceentry.AttachHandler(istio, informer)
			log.Printf("Watching %s.%s across all namespaces with resync period %d and id %q", apiType, kind, resyncPeriod, id)
			informer.Run(ctx.Done())
			return nil
		},
	}

	serve.PersistentFlags().StringVar(&id,
		"id", "istio-cloud-map-operator", "ID of this instance; instances will only ServiceEntries marked with their own ID.")
	serve.PersistentFlags().BoolVar(&debug, "debug", true, "if true, enables more logging")
	serve.PersistentFlags().StringVar(&kubeConfig,
		"kube-config", "", "kubeconfig location; if empty the server will assume it's in a cluster; for local testing use ~/.kube/config")
	serve.PersistentFlags().StringVar(&namespace, "namespace", "",
		"If provided, the namespace this operator publishes CRDs to. If no value is provided it will be populated from the WATCH_NAMESPACE environment variable.")

	// TODO: see if we can derive automatically when we're deployed in AWS
	serve.PersistentFlags().StringVar(&awsRegion, "aws-region", "", "AWS Region to connect to Cloud Map in")

	return serve
}

func main() {
	root := &cobra.Command{
		Short:   "istio-cloud-map",
		Example: "",
	}
	// TODO: add other commands for listing services under management, etc.
	root.AddCommand(serve())
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
