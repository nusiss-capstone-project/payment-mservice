package model

import "time"

// UserPaymentMethod stores a provider payment method bound to a user.
type UserPaymentMethod struct {
	ID                      int64  `gorm:"primaryKey;autoIncrement"`
	UserID                  int64  `gorm:"index"`
	Provider                string `gorm:"size:32;uniqueIndex:uk_provider_pm"`
	ProviderCustomerID      string `gorm:"size:128"`
	ProviderPaymentMethodID string `gorm:"size:128;uniqueIndex:uk_provider_pm"`
	Type                    string `gorm:"size:32"`
	Brand                   string `gorm:"size:32"`
	Last4                   string `gorm:"size:8"`
	Status                  string `gorm:"size:16;index"`
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (UserPaymentMethod) TableName() string {
	return "user_payment_method"
}
