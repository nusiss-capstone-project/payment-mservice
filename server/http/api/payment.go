package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	commonauth "github.com/nusiss-capstone-project/identity-mservice/common/auth"
	"github.com/nusiss-capstone-project/payment-mservice/server/http/data"
	"github.com/nusiss-capstone-project/payment-mservice/server/log"
	"github.com/nusiss-capstone-project/payment-mservice/server/service"
)

// AddPaymentMethod creates a Stripe Checkout setup session and returns the redirect URL.
//
// @Summary Add payment method
// @Description Create Stripe setup checkout session for binding a card. Fails if user already has one.
// @Tags Payment
// @Produce json
// @Success 200 {object} data.BaseResponse{data=data.AddPaymentMethodResponse}
// @Failure 401 {object} data.BaseResponse
// @Failure 409 {object} data.BaseResponse
// @Failure 500 {object} data.BaseResponse
// @Router /payment-ms/v1/web/payment-methods [post]
func AddPaymentMethod(c *gin.Context) {
	userID, ok := commonauth.GetUserID(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, data.BaseResponse{ErrMsg: "unauthorized"})
		return
	}

	resp, err := service.GetPaymentService().AddPaymentMethod(c.Request.Context(), userID)
	if errors.Is(err, service.ErrPaymentMethodAlreadyExists) {
		c.JSON(http.StatusConflict, data.BaseResponse{ErrMsg: err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, data.BaseResponse{ErrMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, data.BaseResponse{Data: resp})
}

// ListPaymentMethods returns the current user's payment methods (at most one in this phase).
//
// @Summary List payment methods
// @Description List bound payment methods for the authenticated user.
// @Tags Payment
// @Produce json
// @Success 200 {object} data.BaseResponse{data=[]data.PaymentMethodVO}
// @Failure 401 {object} data.BaseResponse
// @Failure 500 {object} data.BaseResponse
// @Router /payment-ms/v1/web/payment-methods [get]
func ListPaymentMethods(c *gin.Context) {
	userID, ok := commonauth.GetUserID(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, data.BaseResponse{ErrMsg: "unauthorized"})
		return
	}

	methods, err := service.GetPaymentService().ListPaymentMethods(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, data.BaseResponse{ErrMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, data.BaseResponse{Data: methods})
}

// GetTransaction returns a payment transaction status for the authenticated user.
//
// @Summary Get transaction
// @Description Query transaction success/failure status by payment_id.
// @Tags Payment
// @Produce json
// @Param payment_id path string true "Local payment id"
// @Success 200 {object} data.BaseResponse{data=data.TransactionVO}
// @Failure 401 {object} data.BaseResponse
// @Failure 404 {object} data.BaseResponse
// @Failure 500 {object} data.BaseResponse
// @Router /payment-ms/v1/web/transactions/{payment_id} [get]
func GetTransaction(c *gin.Context) {
	userID, ok := commonauth.GetUserID(c.Request.Context())
	if !ok {
		c.JSON(http.StatusUnauthorized, data.BaseResponse{ErrMsg: "unauthorized"})
		return
	}

	paymentID := c.Param("payment_id")
	tx, err := service.GetPaymentService().GetTransaction(c.Request.Context(), userID, paymentID)
	if errors.Is(err, service.ErrTransactionNotFound) {
		c.JSON(http.StatusNotFound, data.BaseResponse{ErrMsg: err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, data.BaseResponse{ErrMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, data.BaseResponse{Data: tx})
}

// StripeWebhook handles Stripe webhook callbacks.
//
// @Summary Stripe webhook
// @Description Receive Stripe webhook events (checkout.session.completed for setup).
// @Tags Payment
// @Accept json
// @Produce json
// @Success 200 {object} data.BaseResponse
// @Failure 400 {object} data.BaseResponse
// @Failure 500 {object} data.BaseResponse
// @Router /payment-ms/v1/stripe/webhook [post]
func StripeWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, data.BaseResponse{ErrMsg: "read body failed"})
		return
	}
	log.WithContext(c.Request.Context()).Infow("stripe webhook received", "payload", string(payload))
	signature := c.GetHeader("Stripe-Signature")
	if signature == "" {
		c.JSON(http.StatusBadRequest, data.BaseResponse{ErrMsg: "missing Stripe-Signature"})
		return
	}

	if err := service.GetPaymentService().HandleStripeWebhook(c.Request.Context(), payload, signature); err != nil {
		c.JSON(http.StatusBadRequest, data.BaseResponse{ErrMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, data.BaseResponse{Data: "ok"})
}
