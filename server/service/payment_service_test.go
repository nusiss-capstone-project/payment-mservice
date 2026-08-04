package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nusiss-capstone-project/identity-mservice/common/identitypb"
	"github.com/nusiss-capstone-project/payment-mservice/server/config"
	"github.com/nusiss-capstone-project/payment-mservice/server/kafka/producer"
	"github.com/nusiss-capstone-project/payment-mservice/server/log"
	"github.com/nusiss-capstone-project/payment-mservice/server/proxy"
	proxymocks "github.com/nusiss-capstone-project/payment-mservice/server/proxy/mocks"
	daomocks "github.com/nusiss-capstone-project/payment-mservice/server/repository/dao/mocks"
	"github.com/nusiss-capstone-project/payment-mservice/server/repository/model"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockPaymentMethodAddedProducer struct {
	mock.Mock
}

func (m *mockPaymentMethodAddedProducer) PublishPaymentMethodAdded(ctx context.Context, userID int64, provider string) error {
	args := m.Called(ctx, userID, provider)
	return args.Error(0)
}

type mockPaymentTransactionUpdatedProducer struct {
	mock.Mock
}

func (m *mockPaymentTransactionUpdatedProducer) PublishPaymentTransactionUpdated(
	ctx context.Context,
	event producer.PaymentTransactionUpdatedEvent,
) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func initPaymentTestEnv() {
	config.Config = &config.Conf{
		LogConfig: &config.LogConfig{
			Level:    "debug",
			FilePath: "",
		},
		StripeConfig: &config.StripeConfig{
			SetupSuccessURL: "http://localhost:3000/payment/setup",
			SetupCancelURL:  "http://localhost:3000/payment/setup?setup=cancelled",
		},
	}
	log.InitLogger()
}

func newPaymentServiceForTest(
	stripeProxy *proxymocks.StripeProxy,
	userProxy *proxymocks.UserProxy,
	accountDAO *daomocks.PaymentAccountDAO,
	methodDAO *daomocks.PaymentMethodDAO,
	txDAO *daomocks.TransactionDAO,
	methodAddedProducer producer.PaymentMethodAddedProducer,
	txUpdatedProducer producer.PaymentTransactionUpdatedProducer,
) *PaymentServiceImpl {
	if methodAddedProducer == nil {
		methodAddedProducer = new(mockPaymentMethodAddedProducer)
	}
	if txUpdatedProducer == nil {
		txUpdatedProducer = new(mockPaymentTransactionUpdatedProducer)
	}
	if txDAO == nil {
		txDAO = new(daomocks.TransactionDAO)
	}
	return &PaymentServiceImpl{
		stripeProxy:                       stripeProxy,
		userProxy:                         userProxy,
		paymentAccountDAO:                 accountDAO,
		paymentMethodDAO:                  methodDAO,
		transactionDAO:                    txDAO,
		paymentMethodAddedProducer:        methodAddedProducer,
		paymentTransactionUpdatedProducer: txUpdatedProducer,
	}
}

func TestAddPaymentMethod_AlreadyExists(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	userProxy := new(proxymocks.UserProxy)
	accountDAO := new(daomocks.PaymentAccountDAO)
	methodDAO := new(daomocks.PaymentMethodDAO)
	svc := newPaymentServiceForTest(stripeProxy, userProxy, accountDAO, methodDAO, nil, nil, nil)

	methodDAO.On("GetActiveByUserID", mock.Anything, int64(42)).
		Return(&model.UserPaymentMethod{ProviderPaymentMethodID: "pm_existing"}, nil)

	resp, err := svc.AddPaymentMethod(context.Background(), 42)
	require.ErrorIs(t, err, ErrPaymentMethodAlreadyExists)
	require.Nil(t, resp)
	methodDAO.AssertExpectations(t)
}

func TestAddPaymentMethod_CreateCustomerAndSetupSession(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	userProxy := new(proxymocks.UserProxy)
	accountDAO := new(daomocks.PaymentAccountDAO)
	methodDAO := new(daomocks.PaymentMethodDAO)
	svc := newPaymentServiceForTest(stripeProxy, userProxy, accountDAO, methodDAO, nil, nil, nil)

	methodDAO.On("GetActiveByUserID", mock.Anything, int64(42)).Return(nil, nil)
	accountDAO.On("GetByUserIDAndProvider", mock.Anything, int64(42), model.ProviderStripe).Return(nil, nil)
	userProxy.On("GetUser", mock.Anything, int64(42)).Return(&identitypb.GetUserProfileResponse{
		UserId: 42,
		Email:  "u@example.com",
	}, nil)
	stripeProxy.On("CreateCustomer", mock.Anything, proxy.CreateCustomerRequest{
		UserID: "42",
		Email:  "u@example.com",
	}).Return(&proxy.Customer{ID: "cus_1"}, nil)
	accountDAO.On("Create", mock.Anything, mock.MatchedBy(func(a *model.UserPaymentAccount) bool {
		return a.UserID == 42 && a.Provider == model.ProviderStripe && a.ProviderCustomerID == "cus_1"
	})).Return(nil)
	stripeProxy.On("CreatePaymentMethodSetup", mock.Anything, proxy.SetupPaymentMethodRequest{
		UserID:     "42",
		CustomerID: "cus_1",
		SuccessURL: "http://localhost:3000/payment/setup",
		CancelURL:  "http://localhost:3000/payment/setup?setup=cancelled",
	}).Return(&proxy.SetupSession{SessionID: "cs_1", URL: "https://checkout.stripe.com/cs_1"}, nil)

	resp, err := svc.AddPaymentMethod(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "https://checkout.stripe.com/cs_1", resp.RedirectURL)
	mock.AssertExpectationsForObjects(t, stripeProxy, userProxy, accountDAO, methodDAO)
}

func TestAddPaymentMethod_ReuseExistingAccount(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	userProxy := new(proxymocks.UserProxy)
	accountDAO := new(daomocks.PaymentAccountDAO)
	methodDAO := new(daomocks.PaymentMethodDAO)
	svc := newPaymentServiceForTest(stripeProxy, userProxy, accountDAO, methodDAO, nil, nil, nil)

	methodDAO.On("GetActiveByUserID", mock.Anything, int64(7)).Return(nil, nil)
	accountDAO.On("GetByUserIDAndProvider", mock.Anything, int64(7), model.ProviderStripe).
		Return(&model.UserPaymentAccount{UserID: 7, Provider: model.ProviderStripe, ProviderCustomerID: "cus_exist"}, nil)
	stripeProxy.On("CreatePaymentMethodSetup", mock.Anything, mock.MatchedBy(func(req proxy.SetupPaymentMethodRequest) bool {
		return req.UserID == "7" && req.CustomerID == "cus_exist"
	})).Return(&proxy.SetupSession{SessionID: "cs_2", URL: "https://checkout.stripe.com/cs_2"}, nil)

	resp, err := svc.AddPaymentMethod(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "https://checkout.stripe.com/cs_2", resp.RedirectURL)
	userProxy.AssertNotCalled(t, "GetUser", mock.Anything, mock.Anything)
	stripeProxy.AssertNotCalled(t, "CreateCustomer", mock.Anything, mock.Anything)
}

func TestListPaymentMethods(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	userProxy := new(proxymocks.UserProxy)
	accountDAO := new(daomocks.PaymentAccountDAO)
	methodDAO := new(daomocks.PaymentMethodDAO)
	svc := newPaymentServiceForTest(stripeProxy, userProxy, accountDAO, methodDAO, nil, nil, nil)

	now := time.Unix(1700000000, 0).UTC()
	methodDAO.On("ListByUserID", mock.Anything, int64(42)).Return([]*model.UserPaymentMethod{
		{
			ID:                      9,
			ProviderPaymentMethodID: "pm_1",
			Type:                    "card",
			Brand:                   "visa",
			Last4:                   "4242",
			Status:                  model.PaymentMethodStatusActive,
			CreatedAt:               now,
		},
	}, nil)

	got, err := svc.ListPaymentMethods(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int64(9), got[0].ID)
	require.Equal(t, "pm_1", got[0].PaymentMethodID)
	require.Equal(t, "visa", got[0].Brand)
	require.Equal(t, "4242", got[0].Last4)
	require.Equal(t, now.Unix(), got[0].CreatedAt)
}

func TestCreatePayment_Success(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	methodDAO := new(daomocks.PaymentMethodDAO)
	txDAO := new(daomocks.TransactionDAO)
	svc := newPaymentServiceForTest(stripeProxy, new(proxymocks.UserProxy), new(daomocks.PaymentAccountDAO), methodDAO, txDAO, nil, nil)

	txDAO.On("GetByBizID", mock.Anything, "biz-1").Return(nil, nil)
	methodDAO.On("GetByID", mock.Anything, int64(9)).Return(&model.UserPaymentMethod{
		ID:                      9,
		UserID:                  42,
		ProviderCustomerID:      "cus_1",
		ProviderPaymentMethodID: "pm_1",
		Status:                  model.PaymentMethodStatusActive,
	}, nil)
	stripeProxy.On("CreatePayment", mock.Anything, mock.MatchedBy(func(req proxy.CreatePaymentRequest) bool {
		return req.CustomerID == "cus_1" &&
			req.PaymentMethodID == "pm_1" &&
			req.Amount == 1000 &&
			req.Currency == "sgd" &&
			req.Metadata["biz_id"] == "biz-1" &&
			req.Metadata["user_id"] == "42" &&
			req.Metadata["payment_id"] != ""
	})).Return(&proxy.Payment{ID: "pi_1", Status: "succeeded"}, nil)
	txDAO.On("Create", mock.Anything, mock.MatchedBy(func(tx *model.Transaction) bool {
		return tx.BizID == "biz-1" &&
			tx.UserID == 42 &&
			tx.PaymentMethodID == 9 &&
			tx.Amount == 1000 &&
			tx.Currency == "sgd" &&
			tx.ProviderPaymentID == "pi_1" &&
			tx.Status == model.TransactionStatusSucceeded &&
			len(tx.PaymentID) == 16
	})).Return(nil)

	result, err := svc.CreatePayment(context.Background(), CreatePaymentInput{
		BizID:           "biz-1",
		UserID:          42,
		Amount:          1000,
		Currency:        "sgd",
		PaymentMethodID: 9,
	})
	require.NoError(t, err)
	require.Equal(t, model.TransactionStatusSucceeded, result.Status)
	require.Len(t, result.PaymentID, 16)
	mock.AssertExpectationsForObjects(t, stripeProxy, methodDAO, txDAO)
}

func TestCreatePayment_Idempotent(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	txDAO := new(daomocks.TransactionDAO)
	svc := newPaymentServiceForTest(stripeProxy, new(proxymocks.UserProxy), new(daomocks.PaymentAccountDAO), new(daomocks.PaymentMethodDAO), txDAO, nil, nil)

	txDAO.On("GetByBizID", mock.Anything, "biz-1").Return(&model.Transaction{
		PaymentID: "pay_existing____",
		BizID:     "biz-1",
		Status:    model.TransactionStatusSucceeded,
	}, nil)

	result, err := svc.CreatePayment(context.Background(), CreatePaymentInput{
		BizID:           "biz-1",
		UserID:          42,
		Amount:          1000,
		PaymentMethodID: 9,
	})
	require.NoError(t, err)
	require.Equal(t, "pay_existing____", result.PaymentID)
	require.Equal(t, model.TransactionStatusSucceeded, result.Status)
	stripeProxy.AssertNotCalled(t, "CreatePayment", mock.Anything, mock.Anything)
}

func TestGetTransaction(t *testing.T) {
	initPaymentTestEnv()
	txDAO := new(daomocks.TransactionDAO)
	svc := newPaymentServiceForTest(new(proxymocks.StripeProxy), new(proxymocks.UserProxy), new(daomocks.PaymentAccountDAO), new(daomocks.PaymentMethodDAO), txDAO, nil, nil)

	now := time.Unix(1700000000, 0).UTC()
	txDAO.On("GetByPaymentID", mock.Anything, "pay1234567890123").Return(&model.Transaction{
		PaymentID: "pay1234567890123",
		BizID:     "biz-1",
		UserID:    42,
		Amount:    1000,
		Currency:  "sgd",
		Status:    model.TransactionStatusSucceeded,
		CreatedAt: now,
	}, nil)

	got, err := svc.GetTransaction(context.Background(), 42, "pay1234567890123")
	require.NoError(t, err)
	require.Equal(t, "pay1234567890123", got.PaymentID)
	require.Equal(t, model.TransactionStatusSucceeded, got.Status)
}

func TestHandleStripeWebhook_SetupCompleted(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	userProxy := new(proxymocks.UserProxy)
	accountDAO := new(daomocks.PaymentAccountDAO)
	methodDAO := new(daomocks.PaymentMethodDAO)
	eventProducer := new(mockPaymentMethodAddedProducer)
	svc := newPaymentServiceForTest(stripeProxy, userProxy, accountDAO, methodDAO, nil, eventProducer, nil)

	stripeProxy.On("ParseWebhookEvent", []byte("payload"), "sig").Return(&proxy.WebhookEvent{
		EventType:     "checkout.session.completed",
		Mode:          checkoutSessionModeSetup,
		SetupIntentID: "seti_1",
		CustomerID:    "cus_1",
		Metadata:      map[string]string{"user_id": "42"},
	}, nil)
	methodDAO.On("GetActiveByUserID", mock.Anything, int64(42)).Return(nil, nil)
	stripeProxy.On("GetPaymentMethodBySetupIntent", mock.Anything, "seti_1").Return(&proxy.PaymentMethodInfo{
		ID:         "pm_1",
		CustomerID: "cus_1",
		Type:       "card",
		Brand:      "visa",
		Last4:      "4242",
	}, nil)
	methodDAO.On("UpsertByProviderPaymentMethodID", mock.Anything, mock.MatchedBy(func(m *model.UserPaymentMethod) bool {
		return m.UserID == 42 &&
			m.Provider == model.ProviderStripe &&
			m.ProviderCustomerID == "cus_1" &&
			m.ProviderPaymentMethodID == "pm_1" &&
			m.Brand == "visa" &&
			m.Last4 == "4242" &&
			m.Status == model.PaymentMethodStatusActive
	})).Return(nil)
	eventProducer.On("PublishPaymentMethodAdded", mock.Anything, int64(42), model.ProviderStripe).Return(nil)

	err := svc.HandleStripeWebhook(context.Background(), []byte("payload"), "sig")
	require.NoError(t, err)
	mock.AssertExpectationsForObjects(t, stripeProxy, methodDAO, eventProducer)
}

func TestHandleStripeWebhook_IgnoreWhenAlreadyBound(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	methodDAO := new(daomocks.PaymentMethodDAO)
	svc := newPaymentServiceForTest(stripeProxy, new(proxymocks.UserProxy), new(daomocks.PaymentAccountDAO), methodDAO, nil, nil, nil)

	stripeProxy.On("ParseWebhookEvent", []byte("payload"), "sig").Return(&proxy.WebhookEvent{
		EventType:     "checkout.session.completed",
		Mode:          checkoutSessionModeSetup,
		SetupIntentID: "seti_1",
		Metadata:      map[string]string{"user_id": "42"},
	}, nil)
	methodDAO.On("GetActiveByUserID", mock.Anything, int64(42)).
		Return(&model.UserPaymentMethod{ProviderPaymentMethodID: "pm_old"}, nil)

	err := svc.HandleStripeWebhook(context.Background(), []byte("payload"), "sig")
	require.NoError(t, err)
	stripeProxy.AssertNotCalled(t, "GetPaymentMethodBySetupIntent", mock.Anything, mock.Anything)
	methodDAO.AssertNotCalled(t, "UpsertByProviderPaymentMethodID", mock.Anything, mock.Anything)
}

func TestHandleStripeWebhook_IgnoreNonSetupMode(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	methodDAO := new(daomocks.PaymentMethodDAO)
	svc := newPaymentServiceForTest(stripeProxy, new(proxymocks.UserProxy), new(daomocks.PaymentAccountDAO), methodDAO, nil, nil, nil)

	stripeProxy.On("ParseWebhookEvent", []byte("payload"), "sig").Return(&proxy.WebhookEvent{
		EventType: "checkout.session.completed",
		Mode:      "payment",
	}, nil)

	err := svc.HandleStripeWebhook(context.Background(), []byte("payload"), "sig")
	require.NoError(t, err)
	methodDAO.AssertNotCalled(t, "GetActiveByUserID", mock.Anything, mock.Anything)
}

func TestHandleStripeWebhook_ParseError(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	svc := newPaymentServiceForTest(stripeProxy, new(proxymocks.UserProxy), new(daomocks.PaymentAccountDAO), new(daomocks.PaymentMethodDAO), nil, nil, nil)

	stripeProxy.On("ParseWebhookEvent", []byte("bad"), "sig").Return(nil, errors.New("bad signature"))

	err := svc.HandleStripeWebhook(context.Background(), []byte("bad"), "sig")
	require.Error(t, err)
}

func TestHandleStripeWebhook_PaymentIntentSucceeded_SkipKafkaWhenAlreadyTerminal(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	txDAO := new(daomocks.TransactionDAO)
	txProducer := new(mockPaymentTransactionUpdatedProducer)
	svc := newPaymentServiceForTest(stripeProxy, new(proxymocks.UserProxy), new(daomocks.PaymentAccountDAO), new(daomocks.PaymentMethodDAO), txDAO, nil, txProducer)

	stripeProxy.On("ParseWebhookEvent", []byte("payload"), "sig").Return(&proxy.WebhookEvent{
		EventType: "payment_intent.succeeded",
		PaymentID: "pi_1",
		Status:    "succeeded",
		Metadata:  map[string]string{"payment_id": "pay1234567890123"},
	}, nil)
	txDAO.On("GetByPaymentID", mock.Anything, "pay1234567890123").Return(&model.Transaction{
		PaymentID: "pay1234567890123",
		BizID:     "biz-1",
		UserID:    42,
		Provider:  model.ProviderStripe,
		Status:    model.TransactionStatusSucceeded,
	}, nil)

	err := svc.HandleStripeWebhook(context.Background(), []byte("payload"), "sig")
	require.NoError(t, err)
	txDAO.AssertNotCalled(t, "UpdateStatusByPaymentID", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	txProducer.AssertNotCalled(t, "PublishPaymentTransactionUpdated", mock.Anything, mock.Anything)
}

func TestHandleStripeWebhook_PaymentIntentSucceeded_PublishKafkaFromPending(t *testing.T) {
	initPaymentTestEnv()
	stripeProxy := new(proxymocks.StripeProxy)
	txDAO := new(daomocks.TransactionDAO)
	txProducer := new(mockPaymentTransactionUpdatedProducer)
	svc := newPaymentServiceForTest(stripeProxy, new(proxymocks.UserProxy), new(daomocks.PaymentAccountDAO), new(daomocks.PaymentMethodDAO), txDAO, nil, txProducer)

	stripeProxy.On("ParseWebhookEvent", []byte("payload"), "sig").Return(&proxy.WebhookEvent{
		EventType: "payment_intent.succeeded",
		PaymentID: "pi_1",
		Status:    "succeeded",
		Metadata:  map[string]string{"payment_id": "pay1234567890123"},
	}, nil)
	txDAO.On("GetByPaymentID", mock.Anything, "pay1234567890123").Return(&model.Transaction{
		PaymentID: "pay1234567890123",
		BizID:     "biz-1",
		UserID:    42,
		Provider:  model.ProviderStripe,
		Status:    model.TransactionStatusPending,
	}, nil)
	txDAO.On("UpdateStatusByPaymentID", mock.Anything, "pay1234567890123", model.TransactionStatusSucceeded, "").Return(nil)
	txProducer.On("PublishPaymentTransactionUpdated", mock.Anything, mock.MatchedBy(func(e producer.PaymentTransactionUpdatedEvent) bool {
		return e.UserID == 42 &&
			e.PaymentID == "pay1234567890123" &&
			e.BizID == "biz-1" &&
			e.EventType == producer.PaymentSucceededEventType &&
			e.Status == model.TransactionStatusSucceeded
	})).Return(nil)

	err := svc.HandleStripeWebhook(context.Background(), []byte("payload"), "sig")
	require.NoError(t, err)
	mock.AssertExpectationsForObjects(t, stripeProxy, txDAO, txProducer)
}
