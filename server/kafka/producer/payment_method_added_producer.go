package producer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/nusiss-capstone-project/payment-mservice/server/repository/model"
)

const (
	PaymentMethodAddedTopic     = "payment.payment_method.added"
	PaymentMethodAddedEventType = "payment-method-added"
)

type PaymentMethodAddedEvent struct {
	UserID    int64  `json:"user_id"`
	EventType string `json:"event_type"`
	EventTime int64  `json:"event_time"`
	Provider  string `json:"provider"`
}

type PaymentMethodAddedProducer interface {
	PublishPaymentMethodAdded(ctx context.Context, userID int64, provider string) error
}

type paymentMethodAddedProducerImpl struct {
	producer KafkaProducer
	topic    string
}

var (
	paymentMethodAddedProducerOnce sync.Once
	paymentMethodAddedProducerInst PaymentMethodAddedProducer
)

func GetPaymentMethodAddedProducer() PaymentMethodAddedProducer {
	paymentMethodAddedProducerOnce.Do(func() {
		paymentMethodAddedProducerInst = &paymentMethodAddedProducerImpl{
			producer: GetKafkaProducer(),
			topic:    PaymentMethodAddedTopic,
		}
	})
	return paymentMethodAddedProducerInst
}

func (p *paymentMethodAddedProducerImpl) PublishPaymentMethodAdded(
	ctx context.Context,
	userID int64,
	provider string,
) error {
	if userID <= 0 {
		return errors.New("user_id must be positive")
	}
	if provider == "" {
		provider = model.ProviderStripe
	}

	event := PaymentMethodAddedEvent{
		UserID:    userID,
		EventType: PaymentMethodAddedEventType,
		EventTime: time.Now().Unix(),
		Provider:  provider,
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal payment method added event: %w", err)
	}
	return p.producer.Publish(ctx, p.topic, []byte(strconv.FormatInt(userID, 10)), payload)
}
