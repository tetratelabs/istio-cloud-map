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

	"github.com/tetratelabs/istio-cloud-map/pkg/cloudmap"
	"github.com/tetratelabs/istio-cloud-map/pkg/control"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/spf13/cobra"

	"os"

	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	"github.com/tetratelabs/istio-cloud-map/pkg/serviceentry"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	)

	serve = &cobra.Command{
		Use:     "serve",
		Aliases: []string{"serve"},
		Short:   "Starts the Istio Cloud Map Operator server",
		Example: "istio-cloud-map serve --id 123",
		RunE: func(cmd *cobra.Command, args []string) error {

			// the operator-sdk code will panic if we don't set these:
			os.Setenv(k8sutil.OperatorNameEnvVar, id)
			os.Setenv(k8sutil.KubeConfigEnvVar, kubeConfig)
			// we actually configure it to watch all namespaces below by using the empty string, but they have
			// validation that panics if we set this var to the empty string
			if namespace != "" {
				os.Setenv(k8sutil.WatchNamespaceEnvVar, namespace)
			}

			sdk.ExposeMetricsPort()

			k8sutil.AddToSDKScheme(func(scheme *runtime.Scheme) error {
				scheme.AddKnownTypes(
					schema.GroupVersion{
						Group:   apiGroup,
						Version: apiVersion,
					},
					&crd.ServiceEntry{
						TypeMeta: v1.TypeMeta{
							Kind:       "ServiceEntry",
							APIVersion: apiType,
						},
					},
					&crd.ServiceEntryList{},
				)
				return nil
			})

			t := true
			owner := v1.OwnerReference{
				APIVersion: "cloudmap.istio.io",
				Kind:       "ServiceController",
				Name:       id,
				Controller: &t,
			}

			istio := serviceentry.New(owner)
			if debug {
				istio = serviceentry.NewLoggingStore(istio, log.Printf)
			}
			cloudMap := cloudmap.NewStore()

			ctx := context.Background() // common context for cancellation across all loops/routines
			awsRegion := os.Getenv("AWS_REGION")
			if awsRegion == "" {
				log.Fatal("AWS_REGION env var not set, unable to continue")
			}
			log.Printf("Starting Cloud Map watcher in %q", awsRegion)
			cmWatcher, err := cloudmap.NewWatcher(cloudMap, awsRegion)
			if err != nil {
				return err
			}
			go cmWatcher.Run(ctx)

			log.Print("Starting Synchronizer control loop")
			sync, err := control.NewSynchronizer(owner, istio, cloudMap, kubeConfig)
			if err != nil {
				return err
			}
			go sync.Run(ctx)

			log.Printf("Watching %s.%s across all namespaces with resync period %d and id %q", apiType, kind, resyncPeriod, id)
			sdk.Watch(apiType, kind, allNamespaces, resyncPeriod)
			sdk.Handle(serviceentry.NewHandler(istio))
			sdk.Run(ctx)
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
