package hub

type MailMessage struct {
	Action   string      `json:"action"`
	Topic    string      `json:"topic"`
	SubTopic string      `json:"subtopic"`
	Message  interface{} `json:"message"`
}

type ConnMessage struct {
	UserID          string `json:"UserID"`
	AccessToken     string `json:"access_token"`
	ImpersonationID int    `json:"impersonationID"`
	Topics          []string
}
