package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/nusiss-capstone-project/payment-mservice/server/log"
	"github.com/stripe/stripe-go/v75"
	"github.com/stripe/stripe-go/v75/checkout/session"
	"github.com/stripe/stripe-go/v75/customer"
	"github.com/stripe/stripe-go/v75/paymentintent"
	"github.com/stripe/stripe-go/v75/paymentmethod"
	"github.com/stripe/stripe-go/v75/setupintent"
	"github.com/stripe/stripe-go/v75/webhook"
)

type CreateCustomerRequest struct {
	UserID string
	Email  string
}

type Customer struct {
	ID string // cus_xxx
}

type SetupPaymentMethodRequest struct {
	UserID     string
	CustomerID string

	SuccessURL string
	CancelURL  string
}

type SetupSession struct {
	SessionID string
	URL       string
}

type CreatePaymentRequest struct {
	CustomerID string

	PaymentMethodID string

	Amount int64

	Currency string

	Metadata map[string]string
}

type Payment struct {
	ID     string // pi_xxx
	Status string
}

type PaymentMethodInfo struct {
	ID         string // pm_xxx
	CustomerID string
	Type       string
	Brand      string
	Last4      string
}

type WebhookEvent struct {
	EventType string

	PaymentID string

	SetupIntentID string

	PaymentMethodID string

	CustomerID string

	Status string

	Mode string

	Metadata map[string]string
}

type StripeProxy interface {
	CreateCustomer(ctx context.Context, req CreateCustomerRequest) (*Customer, error)

	CreatePaymentMethodSetup(ctx context.Context, req SetupPaymentMethodRequest) (*SetupSession, error)

	CreatePayment(ctx context.Context, req CreatePaymentRequest) (*Payment, error)

	GetPayment(ctx context.Context, paymentID string) (*Payment, error)

	GetPaymentMethodBySetupIntent(ctx context.Context, setupIntentID string) (*PaymentMethodInfo, error)

	ParseWebhookEvent(payload []byte, signature string) (*WebhookEvent, error)
}

type stripeProxyImpl struct {
	webhookSecret string
}

func (s *stripeProxyImpl) CreateCustomer(ctx context.Context, req CreateCustomerRequest) (*Customer, error) {
	log.WithContext(ctx).Infow("stripe create customer request", "req", req)

	params := &stripe.CustomerParams{
		Params: stripe.Params{Context: ctx},
		Email:  stripe.String(req.Email),
	}
	params.AddMetadata("user_id", req.UserID)
	params.AddMetadata("env", currentEnv())

	cus, err := customer.New(params)
	if err != nil {
		log.WithContext(ctx).Errorw("stripe create customer failed", "req", req, "error", err)
		return nil, fmt.Errorf("create stripe customer: %w", err)
	}
	out := &Customer{ID: cus.ID}
	log.WithContext(ctx).Infow("stripe create customer response", "customer", out)
	return out, nil
}

func (s *stripeProxyImpl) CreatePaymentMethodSetup(ctx context.Context, req SetupPaymentMethodRequest) (*SetupSession, error) {
	log.WithContext(ctx).Infow("stripe create payment method setup request", "req", req)

	params := &stripe.CheckoutSessionParams{
		Params:             stripe.Params{Context: ctx},
		Mode:               stripe.String(string(stripe.CheckoutSessionModeSetup)),
		Customer:           stripe.String(req.CustomerID),
		SuccessURL:         stripe.String(req.SuccessURL),
		CancelURL:          stripe.String(req.CancelURL),
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
	}
	params.AddMetadata("env", currentEnv())
	params.AddMetadata("user_id", req.UserID)
	params.SetupIntentData = &stripe.CheckoutSessionSetupIntentDataParams{}
	params.SetupIntentData.AddMetadata("env", currentEnv())
	params.SetupIntentData.AddMetadata("user_id", req.UserID)

	sess, err := session.New(params)
	if err != nil {
		log.WithContext(ctx).Errorw("stripe create payment method setup failed", "req", req, "error", err)
		return nil, fmt.Errorf("create stripe setup session: %w", err)
	}
	out := &SetupSession{
		SessionID: sess.ID,
		URL:       sess.URL,
	}
	log.WithContext(ctx).Infow("stripe create payment method setup response", "session", out)
	return out, nil
}

func (s *stripeProxyImpl) CreatePayment(ctx context.Context, req CreatePaymentRequest) (*Payment, error) {
	log.WithContext(ctx).Infow("stripe create payment request", "req", req)

	currency := req.Currency
	if currency == "" {
		currency = "sgd"
	}

	params := &stripe.PaymentIntentParams{
		Params:        stripe.Params{Context: ctx},
		Amount:        stripe.Int64(req.Amount),
		Currency:      stripe.String(currency),
		Customer:      stripe.String(req.CustomerID),
		PaymentMethod: stripe.String(req.PaymentMethodID),
		Confirm:       stripe.Bool(true),
		OffSession:    stripe.Bool(true),
	}
	params.AddMetadata("env", currentEnv())
	for k, v := range req.Metadata {
		params.AddMetadata(k, v)
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		log.WithContext(ctx).Errorw("stripe create payment failed", "req", req, "error", err)
		return nil, fmt.Errorf("create stripe payment: %w", err)
	}
	out := &Payment{
		ID:     pi.ID,
		Status: string(pi.Status),
	}
	log.WithContext(ctx).Infow("stripe create payment response", "payment", out)
	return out, nil
}

func (s *stripeProxyImpl) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	log.WithContext(ctx).Infow("stripe get payment request", "payment_id", paymentID)

	pi, err := paymentintent.Get(paymentID, &stripe.PaymentIntentParams{
		Params: stripe.Params{Context: ctx},
	})
	if err != nil {
		log.WithContext(ctx).Errorw("stripe get payment failed", "payment_id", paymentID, "error", err)
		return nil, fmt.Errorf("get stripe payment: %w", err)
	}
	out := &Payment{
		ID:     pi.ID,
		Status: string(pi.Status),
	}
	log.WithContext(ctx).Infow("stripe get payment response", "payment", out)
	return out, nil
}

func (s *stripeProxyImpl) GetPaymentMethodBySetupIntent(ctx context.Context, setupIntentID string) (*PaymentMethodInfo, error) {
	log.WithContext(ctx).Infow("stripe get payment method by setup intent request",
		"setup_intent_id", setupIntentID)

	if setupIntentID == "" {
		return nil, fmt.Errorf("setup intent id is required")
	}

	si, err := setupintent.Get(setupIntentID, &stripe.SetupIntentParams{
		Params: stripe.Params{Context: ctx},
	})
	if err != nil {
		log.WithContext(ctx).Errorw("stripe get setup intent failed",
			"setup_intent_id", setupIntentID, "error", err)
		return nil, fmt.Errorf("get setup intent: %w", err)
	}
	if si.PaymentMethod == nil || si.PaymentMethod.ID == "" {
		return nil, fmt.Errorf("setup intent %s has no payment method", setupIntentID)
	}

	pm, err := paymentmethod.Get(si.PaymentMethod.ID, &stripe.PaymentMethodParams{
		Params: stripe.Params{Context: ctx},
	})
	if err != nil {
		log.WithContext(ctx).Errorw("stripe get payment method failed",
			"setup_intent_id", setupIntentID,
			"payment_method_id", si.PaymentMethod.ID,
			"error", err)
		return nil, fmt.Errorf("get payment method: %w", err)
	}

	info := &PaymentMethodInfo{
		ID:   pm.ID,
		Type: string(pm.Type),
	}
	if pm.Customer != nil {
		info.CustomerID = pm.Customer.ID
	} else if si.Customer != nil {
		info.CustomerID = si.Customer.ID
	}
	if pm.Card != nil {
		info.Brand = string(pm.Card.Brand)
		info.Last4 = pm.Card.Last4
	}
	log.WithContext(ctx).Infow("stripe get payment method by setup intent response",
		"setup_intent_id", setupIntentID, "payment_method", info)
	return info, nil
}

func (s *stripeProxyImpl) ParseWebhookEvent(payload []byte, signature string) (*WebhookEvent, error) {
	// signature / webhook secret are sensitive — do not log them
	log.Logger.Infow("stripe parse webhook request", "payload_len", len(payload))

	if s.webhookSecret == "" {
		return nil, fmt.Errorf("stripe webhook secret is not configured")
	}

	event, err := webhook.ConstructEventWithOptions(payload, signature, s.webhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		log.Logger.Errorw("stripe parse webhook failed", "payload_len", len(payload), "error", err)
		return nil, fmt.Errorf("construct stripe webhook event: %w", err)
	}

	out := &WebhookEvent{EventType: string(event.Type)}
	if event.Data == nil {
		log.Logger.Infow("stripe parse webhook response", "event", out)
		return out, nil
	}

	switch {
	case strings.HasPrefix(string(event.Type), "payment_intent."):
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			log.Logger.Errorw("stripe unmarshal payment_intent webhook failed",
				"event_type", event.Type, "error", err)
			return nil, fmt.Errorf("unmarshal payment_intent webhook: %w", err)
		}
		out.PaymentID = pi.ID
		out.Status = string(pi.Status)
		out.Metadata = pi.Metadata
		out.CustomerID = customerIDFromExpandable(pi.Customer)
	case string(event.Type) == "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			log.Logger.Errorw("stripe unmarshal checkout.session webhook failed",
				"event_type", event.Type, "error", err)
			return nil, fmt.Errorf("unmarshal checkout.session webhook: %w", err)
		}
		out.Status = string(sess.Status)
		out.Mode = string(sess.Mode)
		out.Metadata = sess.Metadata
		out.CustomerID = customerIDFromExpandable(sess.Customer)
		if sess.SetupIntent != nil {
			out.SetupIntentID = sess.SetupIntent.ID
		}
		if sess.PaymentIntent != nil {
			out.PaymentID = sess.PaymentIntent.ID
		}
	}

	log.Logger.Infow("stripe parse webhook response", "event", out)
	return out, nil
}

func customerIDFromExpandable(cus *stripe.Customer) string {
	if cus == nil {
		return ""
	}
	return cus.ID
}

func currentEnv() string {
	for _, key := range []string{"APP_ENV", "GO_ENV"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return "local"
}

var (
	stripeProxy         StripeProxy
	stripeProxySyncOnce sync.Once
)

func GetStripeProxy() StripeProxy {
	stripeProxySyncOnce.Do(func() {
		stripeProxy = &stripeProxyImpl{
			webhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		}
	})
	return stripeProxy
}

func InitStripe() {
	key := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	if key == "" {
		panic("STRIPE_SECRET_KEY is required")
	}
	stripe.Key = key
	_ = GetStripeProxy()
}
