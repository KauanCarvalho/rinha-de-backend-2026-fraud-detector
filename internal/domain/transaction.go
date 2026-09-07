package domain

import "time"

type FraudScoreRequest struct {
	ID              string           `json:"id"`
	Transaction     TransactionInfo  `json:"transaction"`
	Customer        CustomerInfo     `json:"customer"`
	Merchant        MerchantInfo     `json:"merchant"`
	Terminal        TerminalInfo     `json:"terminal"`
	LastTransaction *LastTransaction `json:"last_transaction"`
}

type TransactionInfo struct {
	Amount       float64   `json:"amount"`
	Installments int       `json:"installments"`
	RequestedAt  time.Time `json:"requested_at"`
}

type CustomerInfo struct {
	AvgAmount      float64  `json:"avg_amount"`
	TxCount24h     int      `json:"tx_count_24h"`
	KnownMerchants []string `json:"known_merchants"`
}

type MerchantInfo struct {
	ID        string  `json:"id"`
	MCC       string  `json:"mcc"`
	AvgAmount float64 `json:"avg_amount"`
}

type TerminalInfo struct {
	IsOnline    bool    `json:"is_online"`
	CardPresent bool    `json:"card_present"`
	KmFromHome  float64 `json:"km_from_home"`
}

type LastTransaction struct {
	Timestamp     time.Time `json:"timestamp"`
	KmFromCurrent float64   `json:"km_from_current"`
}

type FraudScoreResponse struct {
	Approved   bool    `json:"approved"`
	FraudScore float64 `json:"fraud_score"`
}
