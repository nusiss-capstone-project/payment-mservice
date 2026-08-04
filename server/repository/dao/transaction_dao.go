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

type TransactionDAO interface {
	Create(ctx context.Context, tx *model.Transaction) error
	GetByBizID(ctx context.Context, bizID string) (*model.Transaction, error)
	GetByPaymentID(ctx context.Context, paymentID string) (*model.Transaction, error)
	UpdateStatusByPaymentID(ctx context.Context, paymentID, status, remark string) error
}

type transactionDAOImpl struct {
	db *gorm.DB
}

func (d *transactionDAOImpl) Create(ctx context.Context, tx *model.Transaction) error {
	if err := d.db.WithContext(ctx).Create(tx).Error; err != nil {
		log.WithContext(ctx).Errorw("create transaction failed",
			"payment_id", tx.PaymentID,
			"biz_id", tx.BizID,
			"user_id", tx.UserID,
			"error", err,
		)
		return err
	}
	log.WithContext(ctx).Infow("transaction created",
		"payment_id", tx.PaymentID,
		"biz_id", tx.BizID,
		"user_id", tx.UserID,
		"provider_payment_id", tx.ProviderPaymentID,
		"status", tx.Status,
		"amount", tx.Amount,
		"currency", tx.Currency,
	)
	return nil
}

func (d *transactionDAOImpl) GetByBizID(ctx context.Context, bizID string) (*model.Transaction, error) {
	var tx model.Transaction
	err := d.db.WithContext(ctx).Where("biz_id = ?", bizID).First(&tx).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.WithContext(ctx).Errorw("get transaction by biz_id failed",
			"biz_id", bizID,
			"error", err,
		)
		return nil, err
	}
	return &tx, nil
}

func (d *transactionDAOImpl) GetByPaymentID(ctx context.Context, paymentID string) (*model.Transaction, error) {
	var tx model.Transaction
	err := d.db.WithContext(ctx).Where("payment_id = ?", paymentID).First(&tx).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		log.WithContext(ctx).Errorw("get transaction by payment_id failed",
			"payment_id", paymentID,
			"error", err,
		)
		return nil, err
	}
	return &tx, nil
}

func (d *transactionDAOImpl) UpdateStatusByPaymentID(ctx context.Context, paymentID, status, remark string) error {
	res := d.db.WithContext(ctx).Model(&model.Transaction{}).
		Where("payment_id = ?", paymentID).
		Updates(map[string]any{
			"status": status,
			"remark": remark,
		})
	if res.Error != nil {
		log.WithContext(ctx).Errorw("update transaction status failed",
			"payment_id", paymentID,
			"status", status,
			"error", res.Error,
		)
		return res.Error
	}
	log.WithContext(ctx).Infow("transaction status updated",
		"payment_id", paymentID,
		"status", status,
		"rows", res.RowsAffected,
	)
	return nil
}

var (
	transactionDAO         TransactionDAO
	transactionDAOSyncOnce sync.Once
)

func GetTransactionDAO() TransactionDAO {
	transactionDAOSyncOnce.Do(func() {
		transactionDAO = &transactionDAOImpl{db: repository.DB}
	})
	return transactionDAO
}
