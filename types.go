package hub

// ActionMessage is the message from the network
type ActionMessage struct {
	Action  string      `json:"action"`
	Payload interface{} `json:"payload"`
}

// PublishMessage is the structure used for publishing message.
type PublishMessage struct {
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"`
}

// SubscriptionsMessage is the structure used for subscriptions.
type SubscriptionsMessage struct {
	Topics []string `json:"payload"`
}
