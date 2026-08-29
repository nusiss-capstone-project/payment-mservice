package data

type AddPaymentMethodResponse struct {
	RedirectURL string `json:"redirect_url"`
}

type PaymentMethodVO struct {
	ID              int64  `json:"id"`
	PaymentMethodID string `json:"payment_method_id"`
	Type            string `json:"type"`
	Brand           string `json:"brand"`
	Last4           string `json:"last4"`
	Status          string `json:"status"`
	CreatedAt       int64  `json:"created_at"`
}

type TransactionVO struct {
	PaymentID string `json:"payment_id"`
	BizID     string `json:"biz_id"`
	Status    string `json:"status"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	CreatedAt int64  `json:"created_at"`
}
