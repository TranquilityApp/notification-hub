package hub

// MailMessage is the message from the network
type MailMessage struct {
	Action  string      `json:"action"`
	Topic   string      `json:"topic"`
	Message interface{} `json:"message"`
}
