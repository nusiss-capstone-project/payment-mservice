package model

import "time"

const (
	ProviderStripe = "stripe"

	PaymentMethodStatusActive   = "ACTIVE"
	PaymentMethodStatusInactive = "INACTIVE"
)

// UserPaymentAccount maps a local user to a provider customer (e.g. Stripe cus_xxx).
type UserPaymentAccount struct {
	ID                 int64  `gorm:"primaryKey;autoIncrement"`
	UserID             int64  `gorm:"index"`
	Provider           string `gorm:"size:32;uniqueIndex:uk_provider_customer"`
	ProviderCustomerID string `gorm:"size:128;uniqueIndex:uk_provider_customer"`
	CreatedAt          time.Time
}

func (UserPaymentAccount) TableName() string {
	return "user_payment_account"
}
