package config

import (
	"flag"
	"fmt"

	"github.com/krateoplatformops/plumbing/env"
)

type Configuration struct {
	WatchNamespace   string
	PollingInterval  string
	MaxReconcileRate string
}

func (r *Configuration) String() string {
	return fmt.Sprintf("WATCH_NAMESPACE: %s - MAX_RECONCILE_RATE: %s - POLLING_INTERVAL: %s", r.WatchNamespace, r.MaxReconcileRate, r.PollingInterval)
}

func ParseConfig() Configuration {
	watchNamespace := flag.String("watchnamespace",
		env.String("WATCH_NAMESPACE", ""), "Default namespace to watch")
	maxReconcileRate := flag.String("maxreconcilerate",
		env.String("MAX_RECONCILE_RATE", "1"), "Maximum reconcile rate (default: 1)")
	pollingInterval := flag.String("pollinginterval",
		env.String("POLLING_INTERVAL", "300"), "Polling interval in seconds (default: 300)")

	flag.Parse()

	return Configuration{
		WatchNamespace:   *watchNamespace,
		PollingInterval:  *pollingInterval,
		MaxReconcileRate: *maxReconcileRate,
	}
}
