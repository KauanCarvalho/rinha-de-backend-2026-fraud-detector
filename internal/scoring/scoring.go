package scoring

const K = 5

const Threshold = 0.6

func FraudScore(fraudCount int) float64 {
	return float64(fraudCount) / float64(K)
}

func Approved(fraudScore float64) bool {
	return fraudScore < Threshold
}
