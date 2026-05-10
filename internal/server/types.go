package server

// OutlineWebhookPayload 为 Outline Webhook POST JSON 的常见结构（event + data.id）。
type OutlineWebhookPayload struct {
	Event string `json:"event"`
	Data  struct {
		ID string `json:"id"`
	} `json:"data"`
}
