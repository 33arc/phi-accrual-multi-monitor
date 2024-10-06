package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	PhiGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "server_phivalue",
			Help: "Phi value for server failure detection",
		},
		[]string{"server"},
	)
)

func init() {
	prometheus.MustRegister(PhiGauge)
}
