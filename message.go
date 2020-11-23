package hub

// Message is the message from the network
type Message struct {
	Action string `json:"action"`
	Topic  string `json:"topic"`
}

type SubscriptionsMessage struct {
	Topics []string `json:"topics"`
}
