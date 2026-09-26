package strategy

// Strategy names a delivery strategy.
type Strategy string

const (
	Rolling   Strategy = "rolling"
	Canary    Strategy = "canary"
	BlueGreen Strategy = "blue-green"
)

// RolloutConfig holds per-project rollout policy.
type RolloutConfig struct {
	Strategy           Strategy
	CanarySteps        []int   // traffic-weight steps, e.g. [10, 25, 50, 100]
	AutoAnalysis       bool    // query VictoriaMetrics before each canary promotion
	MetricsURL         string  // VictoriaMetrics base URL for auto-analysis
	ErrorRateThreshold float64 // fraction, e.g. 0.05 = 5%
}

// Default returns a conservative rolling config.
func Default() RolloutConfig {
	return RolloutConfig{
		Strategy:           Rolling,
		CanarySteps:        []int{10, 25, 50, 100},
		AutoAnalysis:       false,
		ErrorRateThreshold: 0.05,
	}
}
