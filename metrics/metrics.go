package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	PhiGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "server_phivalue",
			Help: "Phi value",
		},
		[]string{"server_name"},
	)

	ServerRuntime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "server_runtime_seconds",
			Help: "Server runtime in seconds",
		},
		[]string{"server_name"},
	)
)

func init() {
	prometheus.MustRegister(PhiGauge)
	prometheus.MustRegister(ServerRuntime)
}
