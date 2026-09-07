package detector

import (
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/domain"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/knn"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/scoring"
	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/vectorize"
)

type Detector struct {
	index          *knn.PartitionedIndex
	maxExtraLeaves int
}

func New(index *knn.PartitionedIndex, maxExtraLeaves int) *Detector {
	return &Detector{index: index, maxExtraLeaves: maxExtraLeaves}
}

func (d *Detector) Evaluate(req domain.FraudScoreRequest) domain.FraudScoreResponse {
	vec := vectorize.Vectorize(req)
	query := knn.Quantize(knn.Vector(vec))

	neighbors := d.index.Search(query, scoring.K, d.maxExtraLeaves)
	fraudCount := knn.FraudCount(neighbors)
	score := scoring.FraudScore(fraudCount)

	return domain.FraudScoreResponse{
		Approved:   scoring.Approved(score),
		FraudScore: score,
	}
}
