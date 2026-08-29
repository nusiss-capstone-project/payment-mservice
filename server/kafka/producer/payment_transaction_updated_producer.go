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
	PaymentTransactionUpdatedTopic = "payment.transaction.updated"

	PaymentSucceededEventType = "payment-succeeded"
	PaymentFailedEventType    = "payment-failed"
)

type PaymentTransactionUpdatedEvent struct {
	UserID    int64  `json:"user_id"`
	PaymentID string `json:"payment_id"`
	BizID     string `json:"biz_id"`
	EventType string `json:"event_type"`
	EventTime int64  `json:"event_time"`
	Provider  string `json:"provider"`
	Status    string `json:"status"`
}

type PaymentTransactionUpdatedProducer interface {
	PublishPaymentTransactionUpdated(ctx context.Context, event PaymentTransactionUpdatedEvent) error
}

type paymentTransactionUpdatedProducerImpl struct {
	producer KafkaProducer
	topic    string
}

var (
	paymentTransactionUpdatedProducerOnce sync.Once
	paymentTransactionUpdatedProducerInst PaymentTransactionUpdatedProducer
)

func GetPaymentTransactionUpdatedProducer() PaymentTransactionUpdatedProducer {
	paymentTransactionUpdatedProducerOnce.Do(func() {
		paymentTransactionUpdatedProducerInst = &paymentTransactionUpdatedProducerImpl{
			producer: GetKafkaProducer(),
			topic:    PaymentTransactionUpdatedTopic,
		}
	})
	return paymentTransactionUpdatedProducerInst
}

func (p *paymentTransactionUpdatedProducerImpl) PublishPaymentTransactionUpdated(
	ctx context.Context,
	event PaymentTransactionUpdatedEvent,
) error {
	if event.UserID <= 0 {
		return errors.New("user_id must be positive")
	}
	if event.PaymentID == "" {
		return errors.New("payment_id is required")
	}
	if event.EventType == "" {
		return errors.New("event_type is required")
	}
	if event.Provider == "" {
		event.Provider = model.ProviderStripe
	}
	if event.EventTime == 0 {
		event.EventTime = time.Now().Unix()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal payment transaction updated event: %w", err)
	}
	return p.producer.Publish(ctx, p.topic, []byte(strconv.FormatInt(event.UserID, 10)), payload)
}
