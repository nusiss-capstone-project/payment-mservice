package model

import "time"

const (
	TransactionStatusPending   = "PENDING"
	TransactionStatusSucceeded = "SUCCEEDED"
	TransactionStatusFailed    = "FAILED"
)

// Transaction is a local payment record for a provider charge (e.g. Stripe PaymentIntent).
type Transaction struct {
	ID                int64  `gorm:"primaryKey;autoIncrement"`
	PaymentID         string `gorm:"size:16;uniqueIndex"`
	BizID             string `gorm:"size:128;uniqueIndex"`
	UserID            int64  `gorm:"index"`
	PaymentMethodID   int64
	Amount            int64
	Currency          string `gorm:"size:16"`
	Provider          string `gorm:"size:32"`
	ProviderPaymentID string `gorm:"size:128;index"`
	Status            string `gorm:"size:32;index"`
	Remark            string `gorm:"size:512"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Transaction) TableName() string {
	return "user_payment_transaction"
}
