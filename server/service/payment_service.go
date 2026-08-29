package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/nusiss-capstone-project/payment-mservice/server/config"
	"github.com/nusiss-capstone-project/payment-mservice/server/http/data"
	"github.com/nusiss-capstone-project/payment-mservice/server/kafka/producer"
	"github.com/nusiss-capstone-project/payment-mservice/server/log"
	"github.com/nusiss-capstone-project/payment-mservice/server/proxy"
	"github.com/nusiss-capstone-project/payment-mservice/server/repository/dao"
	"github.com/nusiss-capstone-project/payment-mservice/server/repository/model"
	"github.com/nusiss-capstone-project/payment-mservice/server/util"
)

const checkoutSessionModeSetup = "setup"

var (
	ErrPaymentMethodAlreadyExists = errors.New("payment method already exists")
	ErrTransactionNotFound        = errors.New("transaction not found")
	ErrPaymentMethodNotFound      = errors.New("payment method not found")
	ErrPaymentMethodNotOwned      = errors.New("payment method does not belong to user")
	ErrPaymentMethodInactive      = errors.New("payment method is not active")
)

type CreatePaymentInput struct {
	BizID           string
	UserID          int64
	Amount          int64
	Currency        string
	PaymentMethodID int64
}

type CreatePaymentResult struct {
	PaymentID string
	Status    string
}

type PaymentService interface {
	AddPaymentMethod(ctx context.Context, userID int64) (*data.AddPaymentMethodResponse, error)
	ListPaymentMethods(ctx context.Context, userID int64) ([]*data.PaymentMethodVO, error)
	CreatePayment(ctx context.Context, in CreatePaymentInput) (*CreatePaymentResult, error)
	GetTransaction(ctx context.Context, userID int64, paymentID string) (*data.TransactionVO, error)
	HandleStripeWebhook(ctx context.Context, payload []byte, signature string) error
}

type PaymentServiceImpl struct {
	stripeProxy                       proxy.StripeProxy
	userProxy                         proxy.UserProxy
	paymentAccountDAO                 dao.PaymentAccountDAO
	paymentMethodDAO                  dao.PaymentMethodDAO
	transactionDAO                    dao.TransactionDAO
	paymentMethodAddedProducer        producer.PaymentMethodAddedProducer
	paymentTransactionUpdatedProducer producer.PaymentTransactionUpdatedProducer
}

func (p *PaymentServiceImpl) AddPaymentMethod(ctx context.Context, userID int64) (*data.AddPaymentMethodResponse, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}

	existing, err := p.paymentMethodDAO.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("check existing payment method: %w", err)
	}
	if existing != nil {
		log.WithContext(ctx).Infow("add payment method rejected; already exists",
			"user_id", userID,
			"payment_method_id", existing.ProviderPaymentMethodID,
		)
		return nil, ErrPaymentMethodAlreadyExists
	}

	account, err := p.ensurePaymentAccount(ctx, userID)
	if err != nil {
		return nil, err
	}

	stripeCfg := config.Config.StripeConfig
	if stripeCfg == nil || stripeCfg.SetupSuccessURL == "" || stripeCfg.SetupCancelURL == "" {
		return nil, fmt.Errorf("stripe setup success/cancel urls are not configured")
	}

	session, err := p.stripeProxy.CreatePaymentMethodSetup(ctx, proxy.SetupPaymentMethodRequest{
		UserID:     strconv.FormatInt(userID, 10),
		CustomerID: account.ProviderCustomerID,
		SuccessURL: stripeCfg.SetupSuccessURL,
		CancelURL:  stripeCfg.SetupCancelURL,
	})
	if err != nil {
		log.WithContext(ctx).Errorw("create payment method setup failed",
			"user_id", userID,
			"customer_id", account.ProviderCustomerID,
			"error", err,
		)
		return nil, err
	}

	log.WithContext(ctx).Infow("payment method setup session created",
		"user_id", userID,
		"customer_id", account.ProviderCustomerID,
		"session_id", session.SessionID,
	)
	return &data.AddPaymentMethodResponse{RedirectURL: session.URL}, nil
}

func (p *PaymentServiceImpl) ListPaymentMethods(ctx context.Context, userID int64) ([]*data.PaymentMethodVO, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}

	methods, err := p.paymentMethodDAO.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list payment methods: %w", err)
	}

	out := make([]*data.PaymentMethodVO, 0, len(methods))
	for _, method := range methods {
		out = append(out, toPaymentMethodVO(method))
	}
	return out, nil
}

func (p *PaymentServiceImpl) CreatePayment(ctx context.Context, in CreatePaymentInput) (*CreatePaymentResult, error) {
	if err := validateCreatePaymentInput(in); err != nil {
		return nil, err
	}

	existing, err := p.transactionDAO.GetByBizID(ctx, in.BizID)
	if err != nil {
		return nil, fmt.Errorf("get transaction by biz_id: %w", err)
	}
	if existing != nil {
		log.WithContext(ctx).Infow("create payment idempotent hit",
			"biz_id", in.BizID,
			"payment_id", existing.PaymentID,
			"status", existing.Status,
		)
		return &CreatePaymentResult{
			PaymentID: existing.PaymentID,
			Status:    existing.Status,
		}, nil
	}

	method, err := p.paymentMethodDAO.GetByID(ctx, in.PaymentMethodID)
	if err != nil {
		return nil, fmt.Errorf("get payment method: %w", err)
	}
	if method == nil {
		return nil, ErrPaymentMethodNotFound
	}
	if method.UserID != in.UserID {
		return nil, ErrPaymentMethodNotOwned
	}
	if method.Status != model.PaymentMethodStatusActive {
		return nil, ErrPaymentMethodInactive
	}

	paymentID, err := util.NewPaymentID()
	if err != nil {
		return nil, fmt.Errorf("generate payment id: %w", err)
	}

	currency := strings.ToLower(strings.TrimSpace(in.Currency))
	if currency == "" {
		currency = "sgd"
	}

	stripePayment, err := p.stripeProxy.CreatePayment(ctx, proxy.CreatePaymentRequest{
		CustomerID:      method.ProviderCustomerID,
		PaymentMethodID: method.ProviderPaymentMethodID,
		Amount:          in.Amount,
		Currency:        currency,
		Metadata: map[string]string{
			"payment_id": paymentID,
			"biz_id":     in.BizID,
			"user_id":    strconv.FormatInt(in.UserID, 10),
		},
	})
	if err != nil {
		log.WithContext(ctx).Errorw("stripe create payment failed",
			"biz_id", in.BizID,
			"user_id", in.UserID,
			"payment_id", paymentID,
			"error", err,
		)
		return nil, err
	}
	log.WithContext(ctx).Infow("stripe create payment response", "stripe_payment", stripePayment)

	status := mapStripePaymentStatus(stripePayment.Status)
	tx := &model.Transaction{
		PaymentID:         paymentID,
		BizID:             in.BizID,
		UserID:            in.UserID,
		PaymentMethodID:   in.PaymentMethodID,
		Amount:            in.Amount,
		Currency:          currency,
		Provider:          model.ProviderStripe,
		ProviderPaymentID: stripePayment.ID,
		Status:            status,
	}
	if err := p.transactionDAO.Create(ctx, tx); err != nil {
		return nil, fmt.Errorf("save transaction: %w", err)
	}

	log.WithContext(ctx).Infow("payment created",
		"payment_id", paymentID,
		"biz_id", in.BizID,
		"user_id", in.UserID,
		"provider_payment_id", stripePayment.ID,
		"status", status,
	)

	// Sync RPC: publish Kafka only on success; failures are ignored.
	if status == model.TransactionStatusSucceeded {
		if err := p.paymentTransactionUpdatedProducer.PublishPaymentTransactionUpdated(ctx, producer.PaymentTransactionUpdatedEvent{
			UserID:    in.UserID,
			PaymentID: paymentID,
			BizID:     in.BizID,
			EventType: producer.PaymentSucceededEventType,
			Provider:  model.ProviderStripe,
			Status:    status,
		}); err != nil {
			log.WithContext(ctx).Errorw("publish payment succeeded event failed",
				"payment_id", paymentID,
				"biz_id", in.BizID,
				"error", err,
			)
			return nil, fmt.Errorf("publish payment succeeded event: %w", err)
		}
		log.WithContext(ctx).Infow("payment succeeded event published",
			"payment_id", paymentID,
			"biz_id", in.BizID,
		)
	}

	return &CreatePaymentResult{
		PaymentID: paymentID,
		Status:    status,
	}, nil
}

func (p *PaymentServiceImpl) GetTransaction(ctx context.Context, userID int64, paymentID string) (*data.TransactionVO, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	if paymentID == "" {
		return nil, fmt.Errorf("payment id is required")
	}

	tx, err := p.transactionDAO.GetByPaymentID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("get transaction: %w", err)
	}
	if tx == nil || tx.UserID != userID {
		return nil, ErrTransactionNotFound
	}
	return toTransactionVO(tx), nil
}

func (p *PaymentServiceImpl) HandleStripeWebhook(ctx context.Context, payload []byte, signature string) error {
	event, err := p.stripeProxy.ParseWebhookEvent(ctx, payload, signature)
	if err != nil {
		log.WithContext(ctx).Errorw("parse stripe webhook failed", "error", err)
		return err
	}

	log.WithContext(ctx).Infow("stripe webhook received",
		"event_type", event.EventType,
		"setup_intent_id", event.SetupIntentID,
		"payment_id", event.PaymentID,
		"customer_id", event.CustomerID,
		"mode", event.Mode,
	)

	switch event.EventType {
	case "checkout.session.completed":
		if event.Mode != checkoutSessionModeSetup {
			log.WithContext(ctx).Infow("ignore checkout.session.completed; unsupported mode",
				"mode", event.Mode,
			)
			return nil
		}
		return p.handleSetupSessionCompleted(ctx, event)
	case "payment_intent.succeeded", "payment_intent.payment_failed":
		return p.handlePaymentIntentWebhook(ctx, event)
	default:
		return nil
	}
}

func (p *PaymentServiceImpl) handlePaymentIntentWebhook(ctx context.Context, event *proxy.WebhookEvent) error {
	paymentID := ""
	if event.Metadata != nil {
		paymentID = event.Metadata["payment_id"]
	}
	if paymentID == "" {
		return fmt.Errorf("payment_intent webhook missing metadata.payment_id")
	}

	tx, err := p.transactionDAO.GetByPaymentID(ctx, paymentID)
	if err != nil {
		return fmt.Errorf("get transaction: %w", err)
	}
	if tx == nil {
		log.WithContext(ctx).Infow("ignore payment_intent webhook; transaction not found",
			"payment_id", paymentID,
			"provider_payment_id", event.PaymentID,
		)
		return nil
	}

	newStatus := mapStripePaymentStatus(event.Status)
	if event.EventType == "payment_intent.payment_failed" {
		newStatus = model.TransactionStatusFailed
	}
	if event.EventType == "payment_intent.succeeded" {
		newStatus = model.TransactionStatusSucceeded
	}

	prevStatus := tx.Status
	if prevStatus == newStatus {
		log.WithContext(ctx).Infow("ignore payment_intent webhook; status unchanged",
			"payment_id", paymentID,
			"status", prevStatus,
		)
		return nil
	}

	remark := ""
	if newStatus == model.TransactionStatusFailed {
		remark = event.EventType
	}
	if err := p.transactionDAO.UpdateStatusByPaymentID(ctx, paymentID, newStatus, remark); err != nil {
		return fmt.Errorf("update transaction status: %w", err)
	}

	// Kafka only when sync path had not already reached a terminal result.
	if isTerminalStatus(prevStatus) {
		log.WithContext(ctx).Infow("skip kafka; transaction already terminal from sync path",
			"payment_id", paymentID,
			"prev_status", prevStatus,
			"new_status", newStatus,
		)
		return nil
	}
	if !isTerminalStatus(newStatus) {
		return nil
	}

	eventType := producer.PaymentSucceededEventType
	if newStatus == model.TransactionStatusFailed {
		eventType = producer.PaymentFailedEventType
	}
	if err := p.paymentTransactionUpdatedProducer.PublishPaymentTransactionUpdated(ctx, producer.PaymentTransactionUpdatedEvent{
		UserID:    tx.UserID,
		PaymentID: tx.PaymentID,
		BizID:     tx.BizID,
		EventType: eventType,
		Provider:  tx.Provider,
		Status:    newStatus,
	}); err != nil {
		log.WithContext(ctx).Errorw("publish payment transaction updated failed",
			"payment_id", paymentID,
			"status", newStatus,
			"error", err,
		)
		return fmt.Errorf("publish payment transaction updated: %w", err)
	}
	log.WithContext(ctx).Infow("payment transaction updated event published",
		"payment_id", paymentID,
		"biz_id", tx.BizID,
		"status", newStatus,
		"event_type", eventType,
	)
	return nil
}

func (p *PaymentServiceImpl) handleSetupSessionCompleted(ctx context.Context, event *proxy.WebhookEvent) error {
	if event.SetupIntentID == "" {
		return fmt.Errorf("checkout.session.completed missing setup_intent")
	}

	userID, err := userIDFromMetadata(event.Metadata)
	if err != nil {
		return err
	}

	active, err := p.paymentMethodDAO.GetActiveByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("check existing payment method: %w", err)
	}
	if active != nil {
		log.WithContext(ctx).Infow("ignore setup webhook; user already has active payment method",
			"user_id", userID,
			"payment_method_id", active.ProviderPaymentMethodID,
		)
		return nil
	}

	pm, err := p.stripeProxy.GetPaymentMethodBySetupIntent(ctx, event.SetupIntentID)
	if err != nil {
		log.WithContext(ctx).Errorw("get payment method by setup intent failed",
			"user_id", userID,
			"setup_intent_id", event.SetupIntentID,
			"error", err,
		)
		return err
	}

	customerID := pm.CustomerID
	if customerID == "" {
		customerID = event.CustomerID
	}

	method := &model.UserPaymentMethod{
		UserID:                  userID,
		Provider:                model.ProviderStripe,
		ProviderCustomerID:      customerID,
		ProviderPaymentMethodID: pm.ID,
		Type:                    pm.Type,
		Brand:                   pm.Brand,
		Last4:                   pm.Last4,
		Status:                  model.PaymentMethodStatusActive,
	}
	if err := p.paymentMethodDAO.UpsertByProviderPaymentMethodID(ctx, method); err != nil {
		return fmt.Errorf("save payment method: %w", err)
	}

	if err := p.paymentMethodAddedProducer.PublishPaymentMethodAdded(ctx, userID, method.Provider); err != nil {
		log.WithContext(ctx).Errorw("publish payment method added event failed",
			"user_id", userID,
			"provider", method.Provider,
			"error", err,
		)
		return fmt.Errorf("publish payment method added event: %w", err)
	}
	log.WithContext(ctx).Infow("payment method added event published",
		"user_id", userID,
		"provider", method.Provider,
		"payment_method_id", method.ProviderPaymentMethodID,
	)
	return nil
}

func (p *PaymentServiceImpl) ensurePaymentAccount(ctx context.Context, userID int64) (*model.UserPaymentAccount, error) {
	account, err := p.paymentAccountDAO.GetByUserIDAndProvider(ctx, userID, model.ProviderStripe)
	if err != nil {
		return nil, fmt.Errorf("get payment account: %w", err)
	}
	if account != nil {
		return account, nil
	}

	profile, err := p.userProxy.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user profile: %w", err)
	}

	customer, err := p.stripeProxy.CreateCustomer(ctx, proxy.CreateCustomerRequest{
		UserID: strconv.FormatInt(userID, 10),
		Email:  profile.GetEmail(),
	})
	if err != nil {
		log.WithContext(ctx).Errorw("create stripe customer failed",
			"user_id", userID,
			"error", err,
		)
		return nil, err
	}
	log.WithContext(ctx).Infow("stripe customer created",
		"user_id", userID,
		"customer_id", customer.ID,
	)

	account = &model.UserPaymentAccount{
		UserID:             userID,
		Provider:           model.ProviderStripe,
		ProviderCustomerID: customer.ID,
	}
	if err := p.paymentAccountDAO.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("save payment account: %w", err)
	}
	return account, nil
}

func validateCreatePaymentInput(in CreatePaymentInput) error {
	if strings.TrimSpace(in.BizID) == "" {
		return fmt.Errorf("biz_id is required")
	}
	if in.UserID <= 0 {
		return fmt.Errorf("invalid user id")
	}
	if in.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if in.PaymentMethodID <= 0 {
		return fmt.Errorf("payment_method_id is required")
	}
	return nil
}

func mapStripePaymentStatus(status string) string {
	switch strings.ToLower(status) {
	case "succeeded":
		return model.TransactionStatusSucceeded
	case "canceled", "requires_payment_method":
		return model.TransactionStatusFailed
	default:
		return model.TransactionStatusPending
	}
}

func isTerminalStatus(status string) bool {
	return status == model.TransactionStatusSucceeded || status == model.TransactionStatusFailed
}

func userIDFromMetadata(metadata map[string]string) (int64, error) {
	if metadata == nil {
		return 0, fmt.Errorf("webhook metadata missing user_id")
	}
	raw, ok := metadata["user_id"]
	if !ok || raw == "" {
		return 0, fmt.Errorf("webhook metadata missing user_id")
	}
	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("invalid user_id in webhook metadata: %s", raw)
	}
	return userID, nil
}

func toPaymentMethodVO(method *model.UserPaymentMethod) *data.PaymentMethodVO {
	return &data.PaymentMethodVO{
		ID:              method.ID,
		PaymentMethodID: method.ProviderPaymentMethodID,
		Type:            method.Type,
		Brand:           method.Brand,
		Last4:           method.Last4,
		Status:          method.Status,
		CreatedAt:       method.CreatedAt.Unix(),
	}
}

func toTransactionVO(tx *model.Transaction) *data.TransactionVO {
	return &data.TransactionVO{
		PaymentID: tx.PaymentID,
		BizID:     tx.BizID,
		Status:    tx.Status,
		Amount:    tx.Amount,
		Currency:  tx.Currency,
		CreatedAt: tx.CreatedAt.Unix(),
	}
}

var (
	paymentService         PaymentService
	paymentServiceSyncOnce sync.Once
)

func GetPaymentService() PaymentService {
	paymentServiceSyncOnce.Do(func() {
		paymentService = &PaymentServiceImpl{
			stripeProxy:                       proxy.GetStripeProxy(),
			userProxy:                         proxy.GetUserProxy(),
			paymentAccountDAO:                 dao.GetPaymentAccountDAO(),
			paymentMethodDAO:                  dao.GetPaymentMethodDAO(),
			transactionDAO:                    dao.GetTransactionDAO(),
			paymentMethodAddedProducer:        producer.GetPaymentMethodAddedProducer(),
			paymentTransactionUpdatedProducer: producer.GetPaymentTransactionUpdatedProducer(),
		}
	})
	return paymentService
}
