package hub

import (
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
)

// Broker is the application structure
type Broker struct {
	Hub
}

type BrokerOption func(*Broker)

// Notifier is used to track hub channel actions. This is mainly used for testing in hub_test.go.
type Notifier interface {
	Notify(s string)
}

func WithNotifier(n Notifier) BrokerOption {
	return func(b *Broker) {
		b.notifier = n
	}
}

// NewBroker is a Broker constructor which instantiates the Hub.
func NewBroker(origins []string, opts ...BrokerOption) *Broker {
	broker := &Broker{
		Hub: *NewHub(os.Stdout, origins),
	}

	for _, opt := range opts {
		opt(broker)
	}

	return broker
}

// Hub contains the active subscribers with thread-safe connection management,
// handles subscriptions and broadcasts messages to the subscribers.
type Hub struct {

	// protects connections
	sync.Mutex

	// registered subscribers
	subscribers map[*Subscriber]bool

	// topics with its subscribers (subscribers)
	topics map[string][]*Subscriber

	// logger
	log *log.Logger

	// allowed origins for http requests.
	allowedOrigins []string

	// register requests from subscribers.
	register chan *Subscriber

	// unregister requests from subscribers.
	unregister chan *Subscriber

	// subscribe requests
	subscribe chan *Subscription

	// emit messages from publisher
	emit chan PublishMessage

	notifier Notifier

	OnSubscribe func(*Subscription)
}

// NewHub Instantiates the Hub.
// Adds the factory design pattern as a websocket HTTP connection upgrader.
// Creates the broker's logger
func NewHub(logOutput io.Writer, origins []string) *Hub {
	h := &Hub{
		allowedOrigins: origins,
		register:       make(chan *Subscriber),
		unregister:     make(chan *Subscriber),
		subscribers:        make(map[*Subscriber]bool),
		subscribe:      make(chan *Subscription),
		emit:           make(chan PublishMessage),
		topics:         make(map[string][]*Subscriber),
	}

	h.log = log.New(leveledLogWriter(logOutput), "", log.LstdFlags)

	return h
}

func (h *Hub) getSubscriber(id string) (*Subscriber, bool) {
	subscriber := &Subscriber{}
	for s := range h.subscribers {
		if s.ID == id {
			subscriber = s
		}
	}
	return subscriber, len(subscriber.ID) != 0
}

// ServeHTTP upgrades HTTP connection to a ws/wss connection.
// Sets up the connection and registers it to the hub for
// read/write operations.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// TODO I think origins goes in here instead of nil. Theyre in h.allowedOrigins
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	csws := &subscriberServerWS{Conn: c}

	subscriber := NewSubscriber(csws, h, uuid.New().String())

	h.register <- subscriber

	csws.SetSubscriber(subscriber)

	go csws.listenWrite()
	csws.listenRead()
}

// doRegister prepares the Hub for the connection
func (h *Hub) doRegister(subscriber *Subscriber) {
	h.Lock()
	defer h.Unlock()
	h.Notify("register")
	h.subscribers[subscriber] = true
}

// doSubscribe adds the topic to the connection's slice of topics.
func (h *Hub) doSubscribe(s *Subscription) {
	h.Lock()
	defer h.Unlock()

	h.Notify("subscribe")

	// initialize topic if it doesn't exist yet.
	if _, ok := h.topics[s.Topic]; !ok {
		h.topics[s.Topic] = make([]*Subscriber, 0)
	}

	s.Subscriber.AddTopic(s.Topic)

	// add Subscriber to the hub topic's Subscribers.
	h.topics[s.Topic] = append(h.topics[s.Topic], s.Subscriber)

	h.log.Printf("[DEBUG] Subscriber %s subscribed to topic %s", s.Subscriber.ID, s.Topic)
	if h.OnSubscribe != nil {
		h.OnSubscribe(s)
	}
}

// doUnregister unregisters a connection from the hub.
func (h *Hub) doUnregister(s *Subscriber) {
	h.Lock()
	defer h.Unlock()

	h.Notify("unregister")

	// get the subscriber
	_, ok := h.subscribers[s]
	if !ok {
		h.log.Println("[WARN] cannot unregister connection, it is not registered.")
		return
	}

	h.handleRemoveSubscriber(s)
	s.close()
	delete(h.subscribers, s)
}

// handleRemoveSubscriber handles the removal of deleting a subscriber from the hub.
// This includes deleting the subscriber from the hub's topics map, and
// cleaning up the hub's topics map if a topic has no more subscribers.
func (h *Hub) handleRemoveSubscriber(s *Subscriber) {
	h.deleteTopicSubscriber(s)
	h.handleEmptyTopics(s)
}

// deleteTopic removes the subscriber from topics in the hub.
func (h *Hub) deleteTopicSubscriber(s *Subscriber) {
	// remove each Subscriber from the hub's topic Subscribers
	for i := 0; i < len(s.Topics); i++ {
		subscribers := h.topics[s.Topics[i]]
		foundIdx := -1

		// find the index of the subscriber in the list of Subscribers subscribed to this topic
		for idx, subscriber := range subscribers {
			if subscriber == s {
				foundIdx = idx
				break
			}
		}

		// use the found index to remove this subscriber from the topic's Subscribers
		if foundIdx != -1 {
			h.log.Printf("[DEBUG] Removing subscriber %s from hub topic %s", s.ID, s.Topics[i])
			h.topics[s.Topics[i]] = append(subscribers[:foundIdx], subscribers[foundIdx+1:]...)
		}
	}
}

// handleEmptyTopics iterates through the subscriber's topics, checking to see if the hub has
// any subscribers of that topic left, and if not, the topic is deleted from the hub.
func (h *Hub) handleEmptyTopics(c *Subscriber) {
	for _, topic := range c.Topics {
		// no more Subscribers on topic, remove topic
		if len(h.topics[topic]) == 0 {
			h.log.Printf("[DEBUG] topic %s has no more Subscribers, deleting from hub.", topic)
			delete(h.topics, topic)
		}
	}
}

// doEmit sends message m to all of the channels of the topic's connections.
func (h *Hub) doEmit(m PublishMessage) {
	defer h.Unlock()
	h.Lock()

	h.Notify("emit")

	// get topic subscribers
	subscribers, ok := h.topics[m.Topic]
	if !ok {
		h.log.Println("[DEBUG] there are no subscriptions from:", m.Topic)
		return
	}

	h.log.Printf("[DEBUG] Sending message to topic %v. Subscriber count %d", m.Topic, len(subscribers))
	for _, s := range subscribers {
		s.send <- m.Payload
	}
}

// Publish sends Mailmessage m to the hub's emitter.
func (h *Hub) Publish(m PublishMessage) {
	// notify each subscriber of a topic
	if len(m.Topic) > 0 {
		h.emit <- m
	}
}

func (h *Hub) Notify(s string) {
	if h.notifier != nil {
		h.notifier.Notify(s)
	}
}

// Run starts the Hub.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register: // Registers user to hub
			h.doRegister(c)
		case c := <-h.unregister: // Unregisteres user from hub
			h.doUnregister(c)
		case m := <-h.emit: // Sends message to a mailbox
			h.doEmit(m)
		case s := <-h.subscribe: // Subscribes a user to the hub
			h.doSubscribe(s)
		}
	}
}
