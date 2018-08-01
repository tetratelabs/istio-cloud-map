package main

import (
	"context"

	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/tetratelabs/istio-route53/pkg/serviceentry"
)

const (
	apiVersion   = "networking.istio.io/v1alpha3"
	kind         = "ServiceEntry"
	namespace    = ""
	resyncPeriod = 5
)

func main() {

	cmd := &cobra.Command{}

	sdk.ExposeMetricsPort()

	logrus.Infof("Watching %s, %s, %s, %d", apiVersion, kind, namespace, resyncPeriod)
	sdk.Watch(apiVersion, kind, namespace, resyncPeriod)
	sdk.Handle(serviceentry.NewHandler())
	sdk.Run(context.Background())
}
