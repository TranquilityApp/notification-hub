package hub

type MailMessage struct {
	Action  string `json:"action"`
	Topic   string `json:"topic"`
	Message string `json:"message"`
}

type ConnMessage struct {
	AuthID      string `json:"AuthID"`
	AccessToken string `json:"access_token"`
}
