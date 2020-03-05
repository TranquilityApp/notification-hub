package hub

type MailMessage struct {
	UserID  string `json:"user_id"`
	Action  string `json:"action"`
	Topic   string `json:"topic"`
	Message string `json:"message"`
}

type ConnMessage struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	Topic       string `json:"topic"`
}
