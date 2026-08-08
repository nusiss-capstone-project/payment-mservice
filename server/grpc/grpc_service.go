package grpc

import (
	"context"
	"errors"

	"github.com/nusiss-capstone-project/payment-mservice/common/paymentpb"
	"github.com/nusiss-capstone-project/payment-mservice/server/log"
	"github.com/nusiss-capstone-project/payment-mservice/server/service"
)

type PaymentService struct {
	paymentpb.UnimplementedPaymentServiceServer
}

func (s *PaymentService) SayHello(ctx context.Context, in *paymentpb.HelloRequest) (*paymentpb.HelloResponse, error) {
	log.WithContext(ctx).Infow("grpc say hello received", "name", in.GetName())
	return &paymentpb.HelloResponse{Message: "Hello " + in.GetName()}, nil
}

func (s *PaymentService) CreatePayment(ctx context.Context, in *paymentpb.CreatePaymentRequest) (*paymentpb.CreatePaymentResponse, error) {
	result, err := service.GetPaymentService().CreatePayment(ctx, service.CreatePaymentInput{
		BizID:           in.GetBizId(),
		UserID:          in.GetUserId(),
		Amount:          in.GetAmount(),
		Currency:        in.GetCurrency(),
		PaymentMethodID: in.GetPaymentMethodId(),
	})
	if err != nil {
		code := paymentpb.ErrorCode_ERROR_CODE_INTERNAL
		switch {
		case errors.Is(err, service.ErrPaymentMethodNotFound):
			code = paymentpb.ErrorCode_ERROR_CODE_NOT_FOUND
		case errors.Is(err, service.ErrPaymentMethodNotOwned),
			errors.Is(err, service.ErrPaymentMethodInactive):
			code = paymentpb.ErrorCode_ERROR_CODE_INVALID_ARGUMENT
		case err.Error() == "biz_id is required",
			err.Error() == "invalid user id",
			err.Error() == "amount must be positive",
			err.Error() == "payment_method_id is required":
			code = paymentpb.ErrorCode_ERROR_CODE_INVALID_ARGUMENT
		}
		log.WithContext(ctx).Errorw("grpc create payment failed",
			"biz_id", in.GetBizId(),
			"user_id", in.GetUserId(),
			"error", err,
		)
		return &paymentpb.CreatePaymentResponse{
			BaseInfo: &paymentpb.BaseResponseInfo{
				Code:    code,
				Message: err.Error(),
			},
		}, nil
	}

	return &paymentpb.CreatePaymentResponse{
		PaymentId: result.PaymentID,
		Status:    result.Status,
		BaseInfo: &paymentpb.BaseResponseInfo{
			Code:    paymentpb.ErrorCode_ERROR_CODE_OK,
			Message: "ok",
		},
	}, nil
}
