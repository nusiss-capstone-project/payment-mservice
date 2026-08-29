package dao

import (
	"context"
	"errors"
	"sync"

	"github.com/nusiss-capstone-project/payment-mservice/server/log"
	"github.com/nusiss-capstone-project/payment-mservice/server/repository"
	"github.com/nusiss-capstone-project/payment-mservice/server/repository/model"
	"gorm.io/gorm"
)

type PaymentAccountDAO interface {
	Create(ctx context.Context, account *model.UserPaymentAccount) error
	GetByUserIDAndProvider(ctx context.Context, userID int64, provider string) (*model.UserPaymentAccount, error)
}

type paymentAccountDAOImpl struct {
	db *gorm.DB
}

func (d *paymentAccountDAOImpl) Create(ctx context.Context, account *model.UserPaymentAccount) error {
	if err := d.db.WithContext(ctx).Create(account).Error; err != nil {
		log.WithContext(ctx).Errorw("create payment account failed",
			"user_id", account.UserID,
			"provider", account.Provider,
			"error", err,
		)
		return err
	}
	log.WithContext(ctx).Infow("payment account created",
		"id", account.ID,
		"user_id", account.UserID,
		"provider", account.Provider,
		"provider_customer_id", account.ProviderCustomerID,
	)
	return nil
}

func (d *paymentAccountDAOImpl) GetByUserIDAndProvider(ctx context.Context, userID int64, provider string) (*model.UserPaymentAccount, error) {
	var account model.UserPaymentAccount
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, provider).
		First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.WithContext(ctx).Errorw("get payment account failed",
			"user_id", userID,
			"provider", provider,
			"error", err,
		)
		return nil, err
	}
	return &account, nil
}

var (
	paymentAccountDAO         PaymentAccountDAO
	paymentAccountDAOSyncOnce sync.Once
)

func GetPaymentAccountDAO() PaymentAccountDAO {
	paymentAccountDAOSyncOnce.Do(func() {
		paymentAccountDAO = &paymentAccountDAOImpl{db: repository.DB}
	})
	return paymentAccountDAO
}
