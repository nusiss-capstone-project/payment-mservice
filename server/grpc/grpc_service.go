package grpc

import (
	"context"

	"github.com/nusiss-capstone-project/payment-mservice/common/paymentpb"
	"github.com/nusiss-capstone-project/payment-mservice/server/log"
)

type PaymentService struct {
	paymentpb.UnimplementedPaymentServiceServer
}

func (s *PaymentService) SayHello(ctx context.Context, in *paymentpb.HelloRequest) (*paymentpb.HelloResponse, error) {
	log.Logger.Infof("Received: %v", in.GetName())
	return &paymentpb.HelloResponse{Message: "Hello " + in.GetName()}, nil
}
