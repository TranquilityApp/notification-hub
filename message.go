package hub

type MailMessage struct {
	Action          string `json:"action"`
	Topic           string `json:"topic"`
	SourceUser      string `json:"sourceUser"`
	DestinationUser string `json:"destinationUser"`
	SubTopic        string `json:"subtopic"`
	Message         string `json:"message"`
}

type ConnMessage struct {
	AuthID          string `json:"AuthID"`
	AccessToken     string `json:"access_token"`
	ImpersonationID int    `json:"impersonationID"`
}
