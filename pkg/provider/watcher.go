package provider

import "context"

// Watcher is the interface of each provider's entrypoint
type Watcher interface {
	Run(ctx context.Context)
}
