package main

import (
	"context"
	"log"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/spf13/cobra"

	"os"

	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	"github.com/tetratelabs/istio-route53/pkg/serviceentry"
	"istio.io/istio/pilot/pkg/config/kube/crd"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	apiGroup     = "networking.istio.io"
	apiVersion   = "v1alpha3"
	apiType      = apiGroup + "/" + apiVersion
	kind         = "ServiceEntry"
	namespace    = ""
	resyncPeriod = 30
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
		Short:   "Starts the Istio-Route53 Operator server",
		Example: "istio-route53 serve --id 123",
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
					schema.GroupVersion{Group: apiGroup, Version: apiVersion},
					&crd.ServiceEntry{
						TypeMeta: v1.TypeMeta{Kind: "ServiceEntry", APIVersion: apiType},
					},
					&crd.ServiceEntryList{},
				)
				return nil
			})

			store := serviceentry.New(id)
			if debug {
				store = serviceentry.NewLoggingStore(store, log.Printf)
			}

			log.Printf("Watching %s, %s, %s, %d with id %q", apiType, kind, "", resyncPeriod, id)
			sdk.Watch(apiType, kind, "", resyncPeriod)
			sdk.Handle(serviceentry.NewHandler(store))
			sdk.Run(context.Background())
			return nil
		},
	}

	serve.PersistentFlags().StringVar(&id,
		"id", "istio-route53-controller", "ID of this instance; instances will only ServiceEntries marked with their own ID.")
	serve.PersistentFlags().BoolVar(&debug, "debug", true, "if true, enables more logging")
	serve.PersistentFlags().StringVar(&kubeConfig,
		"kube-config", "", "kubeconfig location; if empty the server will assume it's in a cluster; for local testing use ~/.kube/config")
	serve.PersistentFlags().StringVar(&namespace, "namespace", "",
		"If provided, the namespace this operator publishes CRDs to. If no value is provided it will be populated from the WATCH_NAMESPACE environment variable.")
	return serve
}

func main() {
	root := &cobra.Command{
		Short:   "istio-route53",
		Example: "",
	}
	// TODO: add other commands for listing services under management, etc.
	root.AddCommand(serve())
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
