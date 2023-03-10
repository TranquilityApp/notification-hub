package hub

// Subscription represents a 1:1 relationship between topic and Subscriber.
type Subscription struct {
	Topic  string
	Subscriber *Subscriber
}

// Subscriber represents a single connection from a user.
type Subscriber struct {
	ID     string
	ws     *subscriberServerWS
	hub    *Hub
	closed bool
	send   chan []byte
	Topics []string
}

// NewSubscriber creates a new Subscriber.
func NewSubscriber(ws *subscriberServerWS, h *Hub, ID string) *Subscriber {
	return &Subscriber{
		ID:   ID,
		send: make(chan []byte, 256),
		ws:   ws,
		hub:  h,
	}
}

// AddTopic adds a topic to a Subscriber.
func (s *Subscriber) AddTopic(topic string) {
	s.Topics = append(s.Topics, topic)
}

// RemoveTopic removes topics from a Subscriber and the Subscriber from hub's topic map
func (s *Subscriber) ClearTopics() {
	s.hub.Lock()
	defer s.hub.Unlock()
	s.hub.handleRemoveSubscriber(s) // removes Subscriber from hub topics and cleans topics up
	s.Topics = nil
}

// Subscribe subscribes a Subscriber to a topic.
func (s *Subscriber) Subscribe(topic string) {
	sub := &Subscription{
		Topic:  topic,
		Subscriber: s,
	}
	s.hub.subscribe <- sub
}

// SubscribeMultiple subscribes the Subscriber to multiple topics.
func (s *Subscriber) SubscribeMultiple(topics []string) {
	for _, topic := range topics {
		c.Subscribe(topic)
	}
}

// close closes the websocket and the send channel.
func (s *Subscriber) close() {
	if !s.closed {
		if s.ws == nil {
			s.hub.log.Println("[DEBUG] websocket was nil")
		} else if err := s.ws.Close(); err != nil {
			s.hub.log.Println("[DEBUG] websocket was already closed:", err)
		} else {
			s.hub.log.Println("[DEBUG] websocket closed.")
			s.hub.log.Println("[DEBUG] closing connection's send channel.")
			close(s.send)
		}
		s.closed = true
	}
}