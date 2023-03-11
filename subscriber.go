package hub

import "nhooyr.io/websocket"

// Subscription represents a 1:1 relationship between topic and Subscriber.
type Subscription struct {
	Topic      string
	Subscriber *Subscriber
}

// Subscriber represents a single connection from a user.
type Subscriber struct {
	ID        string
	ws        *subscriberServerWS
	hub       *Hub
	msgs      chan []byte
	Topics    []string
	closeSlow func()
}

// NewSubscriber creates a new Subscriber.
func NewSubscriber(ws *subscriberServerWS, h *Hub, ID string) *Subscriber {
	return &Subscriber{
		ID:   ID,
		msgs: make(chan []byte, 256),
		ws:   ws,
		hub:  h,
		closeSlow: func() {
			ws.Conn.Close(websocket.StatusPolicyViolation, "connection too slow to keep up with messages")
		},
	}
}

// ClearTopics removes topics from a Subscriber and the Subscriber from hub's topic map
func (s *Subscriber) ClearTopics() {
	s.hub.subscribersMu.Lock()
	defer s.hub.subscribersMu.Unlock()
	if len(s.Topics) > 0 {
		s.hub.deleteTopicsFromSubscriber(s)
		s.hub.handleEmptyTopics()
	}
	s.Topics = nil
}

// Subscribe subscribes a Subscriber to a topic.
func (s *Subscriber) Subscribe(topic string) {
	s.Topics = append(s.Topics, topic)
	sub := &Subscription{
		Topic:      topic,
		Subscriber: s,
	}
	s.hub.subscribe(sub)
}

// SubscribeMultiple subscribes the Subscriber to multiple topics.
func (s *Subscriber) SubscribeMultiple(topics []string) {
	for _, topic := range topics {
		s.Subscribe(topic)
	}
}
