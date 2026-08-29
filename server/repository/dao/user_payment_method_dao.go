package dao

import (
	"context"
	"errors"
	"sync"

	"github.com/nusiss-capstone-project/payment-mservice/server/log"
	"github.com/nusiss-capstone-project/payment-mservice/server/repository"
	"github.com/nusiss-capstone-project/payment-mservice/server/repository/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaymentMethodDAO interface {
	Create(ctx context.Context, method *model.UserPaymentMethod) error
	UpsertByProviderPaymentMethodID(ctx context.Context, method *model.UserPaymentMethod) error
	ListByUserID(ctx context.Context, userID int64) ([]*model.UserPaymentMethod, error)
	GetActiveByUserID(ctx context.Context, userID int64) (*model.UserPaymentMethod, error)
	GetByID(ctx context.Context, id int64) (*model.UserPaymentMethod, error)
}

type paymentMethodDAOImpl struct {
	db *gorm.DB
}

func (d *paymentMethodDAOImpl) Create(ctx context.Context, method *model.UserPaymentMethod) error {
	if err := d.db.WithContext(ctx).Create(method).Error; err != nil {
		log.WithContext(ctx).Errorw("create payment method failed",
			"user_id", method.UserID,
			"provider_payment_method_id", method.ProviderPaymentMethodID,
			"error", err,
		)
		return err
	}
	log.WithContext(ctx).Infow("payment method created",
		"id", method.ID,
		"user_id", method.UserID,
		"provider_payment_method_id", method.ProviderPaymentMethodID,
		"status", method.Status,
	)
	return nil
}

func (d *paymentMethodDAOImpl) UpsertByProviderPaymentMethodID(ctx context.Context, method *model.UserPaymentMethod) error {
	if err := d.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "provider"},
			{Name: "provider_payment_method_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id",
			"provider_customer_id",
			"type",
			"brand",
			"last4",
			"status",
			"updated_at",
		}),
	}).Create(method).Error; err != nil {
		log.WithContext(ctx).Errorw("upsert payment method failed",
			"user_id", method.UserID,
			"provider_payment_method_id", method.ProviderPaymentMethodID,
			"error", err,
		)
		return err
	}
	log.WithContext(ctx).Infow("payment method upserted",
		"id", method.ID,
		"user_id", method.UserID,
		"provider_payment_method_id", method.ProviderPaymentMethodID,
		"brand", method.Brand,
		"last4", method.Last4,
		"status", method.Status,
	)
	return nil
}

func (d *paymentMethodDAOImpl) ListByUserID(ctx context.Context, userID int64) ([]*model.UserPaymentMethod, error) {
	var methods []*model.UserPaymentMethod
	err := d.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id asc").
		Find(&methods).Error
	if err != nil {
		log.WithContext(ctx).Errorw("list payment methods failed",
			"user_id", userID,
			"error", err,
		)
		return nil, err
	}
	return methods, nil
}

func (d *paymentMethodDAOImpl) GetActiveByUserID(ctx context.Context, userID int64) (*model.UserPaymentMethod, error) {
	var method model.UserPaymentMethod
	err := d.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, model.PaymentMethodStatusActive).
		First(&method).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.WithContext(ctx).Errorw("get active payment method failed",
			"user_id", userID,
			"error", err,
		)
		return nil, err
	}
	return &method, nil
}

func (d *paymentMethodDAOImpl) GetByID(ctx context.Context, id int64) (*model.UserPaymentMethod, error) {
	var method model.UserPaymentMethod
	err := d.db.WithContext(ctx).Where("id = ?", id).First(&method).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.WithContext(ctx).Errorw("get payment method by id failed",
			"id", id,
			"error", err,
		)
		return nil, err
	}
	return &method, nil
}

var (
	paymentMethodDAO         PaymentMethodDAO
	paymentMethodDAOSyncOnce sync.Once
)

func GetPaymentMethodDAO() PaymentMethodDAO {
	paymentMethodDAOSyncOnce.Do(func() {
		paymentMethodDAO = &paymentMethodDAOImpl{db: repository.DB}
	})
	return paymentMethodDAO
}
