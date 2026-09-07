package vectorize

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/KauanCarvalho/rinha-de-backend-2026-fraud-detector/internal/domain"
)

const Dim = 14

type Vector [Dim]float64

const (
	DimAmount = iota
	DimInstallments
	DimAmountVsAvg
	DimHourOfDay
	DimDayOfWeek
	DimMinutesSinceLastTx
	DimKmFromLastTx
	DimKmFromHome
	DimTxCount24h
	DimIsOnline
	DimCardPresent
	DimUnknownMerchant
	DimMCCRisk
	DimMerchantAvgAmount
)

const noHistory = -1

const (
	maxHourOfDay    = 23
	daysPerWeek     = 7
	maxWeekdayIndex = daysPerWeek - 1
)

//go:embed resources/normalization.json
var normalizationJSON []byte

//go:embed resources/mcc_risk.json
var mccRiskJSON []byte

// normalization holds the constants from resources/normalization.json.
// These values are part of the challenge's fixed contract and do not change
// during a test run, so they are embedded at build time.
type normalization struct {
	MaxAmount            float64 `json:"max_amount"`
	MaxInstallments      float64 `json:"max_installments"`
	AmountVsAvgRatio     float64 `json:"amount_vs_avg_ratio"`
	MaxMinutes           float64 `json:"max_minutes"`
	MaxKm                float64 `json:"max_km"`
	MaxTxCount24h        float64 `json:"max_tx_count_24h"`
	MaxMerchantAvgAmount float64 `json:"max_merchant_avg_amount"`
}

// defaultMCCRisk is used when merchant.mcc is not present in the risk table.
const defaultMCCRisk = 0.5

// norm and mccRisk are populated once, in init below, from the embedded
// (and therefore immutable for the process's lifetime) config files —
// read-only globals for genuinely constant data, not mutable shared state.
//
//nolint:gochecknoglobals // read-only after init; see comment above
var (
	norm    normalization
	mccRisk map[string]float64
)

// init is the standard, simplest way to validate go:embed-ed data once at
// program start and fail fast (via panic) if it's malformed — there's no
// request to fail instead, and every call site would otherwise need to
// handle an error that can only ever happen due to a build-time mistake.
//
//nolint:gochecknoinits // validates embedded config once at startup, see comment above
func init() {
	if err := json.Unmarshal(normalizationJSON, &norm); err != nil {
		panic(fmt.Sprintf("vectorize: invalid embedded normalization.json: %v", err))
	}
	if err := json.Unmarshal(mccRiskJSON, &mccRisk); err != nil {
		panic(fmt.Sprintf("vectorize: invalid embedded mcc_risk.json: %v", err))
	}
}

// clamp restricts x to the interval [0.0, 1.0].
func clamp(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	default:
		return x
	}
}

// safeRatio returns num/den clamped to [0, 1], treating a non-positive or
// non-finite denominator as "maximum risk" (1.0) when num is positive, and
// 0.0 when num is also non-positive. The challenge spec does not define
// this edge case explicitly.
func safeRatio(num, den float64) float64 {
	if den <= 0 {
		if num > 0 {
			return 1
		}
		return 0
	}
	return clamp(num / den)
}

// Vectorize converts a fraud-score request into its 14-dimensional
// normalized vector.
func Vectorize(req domain.FraudScoreRequest) Vector {
	var v Vector

	v[DimAmount] = safeRatio(req.Transaction.Amount, norm.MaxAmount)
	v[DimInstallments] = safeRatio(float64(req.Transaction.Installments), norm.MaxInstallments)
	v[DimAmountVsAvg] = clamp(amountVsAvg(req.Transaction.Amount, req.Customer.AvgAmount) / norm.AmountVsAvgRatio)

	hour := req.Transaction.RequestedAt.UTC().Hour()
	v[DimHourOfDay] = clamp(float64(hour) / maxHourOfDay)

	weekday := (int(req.Transaction.RequestedAt.UTC().Weekday()) + maxWeekdayIndex) % daysPerWeek
	v[DimDayOfWeek] = clamp(float64(weekday) / maxWeekdayIndex)

	if req.LastTransaction == nil {
		v[DimMinutesSinceLastTx] = noHistory
		v[DimKmFromLastTx] = noHistory
	} else {
		minutes := req.Transaction.RequestedAt.Sub(req.LastTransaction.Timestamp).Minutes()
		v[DimMinutesSinceLastTx] = clamp(minutes / norm.MaxMinutes)
		v[DimKmFromLastTx] = clamp(req.LastTransaction.KmFromCurrent / norm.MaxKm)
	}

	v[DimKmFromHome] = clamp(req.Terminal.KmFromHome / norm.MaxKm)
	v[DimTxCount24h] = clamp(float64(req.Customer.TxCount24h) / norm.MaxTxCount24h)
	v[DimIsOnline] = boolToFloat(req.Terminal.IsOnline)
	v[DimCardPresent] = boolToFloat(req.Terminal.CardPresent)
	v[DimUnknownMerchant] = boolToFloat(!isKnownMerchant(req.Merchant.ID, req.Customer.KnownMerchants))
	v[DimMCCRisk] = mccRiskFor(req.Merchant.MCC)
	v[DimMerchantAvgAmount] = clamp(req.Merchant.AvgAmount / norm.MaxMerchantAvgAmount)

	return v
}

// amountVsAvg returns amount/avgAmount, the raw (unclamped) ratio consumed
// by DimAmountVsAvg. The spec does not define the avgAmount<=0 edge case:
// a non-positive average with a positive amount is treated as maximum risk,
// and a non-positive average with a zero amount as no risk.
func amountVsAvg(amount, avgAmount float64) float64 {
	if avgAmount <= 0 {
		if amount > 0 {
			return norm.AmountVsAvgRatio // dividing by it below yields 1.0
		}
		return 0
	}
	return amount / avgAmount
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func isKnownMerchant(merchantID string, known []string) bool {
	return slices.Contains(known, merchantID)
}

func mccRiskFor(mcc string) float64 {
	if risk, ok := mccRisk[mcc]; ok {
		return risk
	}
	return defaultMCCRisk
}
