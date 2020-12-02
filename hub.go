package hub

import (
	"github.com/TranquilityApp/middleware"

	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
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

// Hub contains the active clients with thread-safe connection management,
// handles subscriptions and broadcasts messages to the clients.
type Hub struct {

	// protects connections
	sync.Mutex

	// registered clients
	clients map[*Client]bool

	// topics with its subscribers (clients)
	topics map[string][]*Client

	// logger
	log *log.Logger

	// allowed origins for http requests.
	allowedOrigins []string

	// register requests from clients.
	register chan *Client

	// unregister requests from clients.
	unregister chan *Client

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
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		clients:        make(map[*Client]bool),
		subscribe:      make(chan *Subscription),
		emit:           make(chan PublishMessage),
		topics:         make(map[string][]*Client),
	}

	h.log = log.New(leveledLogWriter(logOutput), "", log.LstdFlags)

	return h
}

// ServeHTTP upgrades HTTP connection to a ws/wss connection.
// Sets up the connection and registers it to the hub for
// read/write operations.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	ws, err := newClientServerWS(w, r, h.allowedOrigins)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	userID := strings.Split(r.Context().Value(middleware.AuthKey).(string), "|")[1]

	client := NewClient(ws, h, userID)

	h.register <- client

	ws.SetClient(client)

	go ws.listenWrite()
	ws.listenRead()
}

func (h *Hub) getClient(id string) (*Client, bool) {
	client := &Client{}
	for c, _ := range h.clients {
		if c.ID == id {
			client = c
		}
	}
	return client, len(client.ID) != 0
}

// doRegister prepares the Hub for the connection
func (h *Hub) doRegister(client *Client) {
	h.Lock()
	defer h.Unlock()
	h.Notify("register")
	h.clients[client] = true
}

// doSubscribe adds the topic to the connection's slice of topics.
func (h *Hub) doSubscribe(s *Subscription) {
	h.Lock()
	defer h.Unlock()

	h.Notify("subscribe")

	// initialize topic if it doesn't exist yet.
	if _, ok := h.topics[s.Topic]; !ok {
		h.topics[s.Topic] = make([]*Client, 0)
	}

	s.Client.AddTopic(s.Topic)

	// add Client to the hub topic's Clients.
	h.topics[s.Topic] = append(h.topics[s.Topic], s.Client)

	h.log.Printf("[DEBUG] Client %s subscribed to topic %s", s.Client.ID, s.Topic)
	if h.OnSubscribe != nil {
		h.OnSubscribe(s)
	}
}

// doUnregister unregisters a connection from the hub.
func (h *Hub) doUnregister(c *Client) {
	h.Lock()
	defer h.Unlock()

	h.Notify("unregister")

	// get the subscriber
	_, ok := h.clients[c]
	if !ok {
		h.log.Println("[WARN] cannot unregister connection, it is not registered.")
		return
	}

	h.handleRemoveClient(c)
	c.close()
	delete(h.clients, c)
}

// handleRemoveClient handles the removal of deleting a client from the hub.
// This includes deleting the client from the hub's topics map, and
// cleaning up the hub's topics map if a topic has no more clients.
func (h *Hub) handleRemoveClient(c *Client) {
	h.deleteTopicClient(c)
	h.handleEmptyTopics(c)
}

// deleteTopic removes the client from topics in the hub.
func (h *Hub) deleteTopicClient(c *Client) {
	// remove each Client from the hub's topic clients
	for i := 0; i < len(c.Topics); i++ {
		clients := h.topics[c.Topics[i]]
		foundIdx := -1

		// find the index of the client in the list of clients subscribed to this topic
		for idx, client := range clients {
			if client == c {
				foundIdx = idx
				break
			}
		}

		// use the found index to remove this client from the topic's clients
		if foundIdx != -1 {
			h.log.Printf("[DEBUG] Removing client %s from hub topic %s", c.ID, c.Topics[i])
			h.topics[c.Topics[i]] = append(clients[:foundIdx], clients[foundIdx+1:]...)
		}
	}
}

// handleEmptyTopics iterates through the client's topics, checking to see if the hub has
// any subscribers of that topic left, and if not, the topic is deleted from the hub.
func (h *Hub) handleEmptyTopics(c *Client) {
	for _, topic := range c.Topics {
		// no more clients on topic, remove topic
		if len(h.topics[topic]) == 0 {
			h.log.Printf("[DEBUG] topic %s has no more clients, deleting from hub.", topic)
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
	clients, ok := h.topics[m.Topic]
	if !ok {
		h.log.Println("[DEBUG] there are no subscriptions from:", m.Topic)
		return
	}

	h.log.Printf("[DEBUG] Sending message to topic %v. Client count %d", m.Topic, len(clients))
	for _, c := range clients {
		c.send <- m.Payload
	}
}

// Publish sends Mailmessage m to the hub's emitter.
func (h *Hub) Publish(m PublishMessage) {
	// notify each subscriber of a topic
	if len(m.Topic) > 0 {
		h.emit <- m
	}
	return
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
