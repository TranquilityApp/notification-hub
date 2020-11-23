package hub

import (
	"github.com/truescotian/pubsub/internal/pkg/websocket"

	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

// Application is the main app structure
type Application struct {
	Hub
}

// NewApp is an Application constructor which instantiates the Hub.
func NewApp() *Application {
	app := &Application{
		Hub: *NewHub(os.Stdout, "*"),
	}
	return app
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
}

// NewHub Instantiates the Hub.
// Adds the factory design pattern as a websocket HTTP connection upgrader.
// Creates the application's logger
func NewHub(logOutput io.Writer, origins ...string) *Hub {
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
	// only allow GET request
	if r.Method != "GET" {
		http.Error(
			w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed,
		)
		return
	}

	// upgrade the connection
	ws, err := websocket.Upgrade(w, r, h.allowedOrigins)
	if err != nil {
		h.log.Println("[ERROR] failed to upgrade connection:", err)
		return
	}

	client := NewClient(ws, h)

	h.register <- client

	go client.listenWrite()
	client.listenRead()
}

// doRegister prepares the Hub for the connection
func (h *Hub) doRegister(client *Client) {
	h.Lock()
	defer h.Unlock()
	h.clients[client] = true
}

// doSubscribe adds the topic to the connection's slice of topics.
func (h *Hub) doSubscribe(s *Subscription) {
	h.Lock()
	defer h.Unlock()

	// initialize topic if it doesn't exist yet.
	if _, ok := h.topics[s.Topic]; !ok {
		h.topics[s.Topic] = make([]*Client, 0)
	}

	s.Client.AddTopic(s.Topic)

	// add Client to the hub topic's Clients.
	h.topics[s.Topic] = append(h.topics[s.Topic], s.Client)

	h.log.Printf("[DEBUG] Client %s subscribed to topic %s", s.Client.ID, s.Topic)
}

// doUnregister unregisters a connection from the hub.
func (h *Hub) doUnregister(c *Client) {
	h.Lock()
	defer h.Unlock()

	h.log.Println("[DEBUG] Unregistering connection.")

	// get the subscriber
	_, ok := h.clients[c]
	if !ok {
		h.log.Println("[WARN] cannot unregister connection, it is not registered.")
		return
	}

	h.deleteTopic(c)
	c.close()
	delete(h.clients, c)
}

// deleteTopic removes the client from topics in the hub. If the topic has no more clients,
// then the topic is removed from the hub.
func (h *Hub) deleteTopic(c *Client) {
	// remove each connection from the hub's topic connections
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

		// use the found index to remove this connection from the topic's connections
		if foundIdx != -1 {
			h.log.Println("[DEBUG] removing client %v from hub's topic %v", c.ID, c.Topics[i])
			h.topics[c.Topics[i]] = append(clients[:foundIdx], clients[foundIdx+1:]...)
		}
	}

	// check if the topic needs to be deleted from the hub by cycling through this subscriber's
	// topics.
	for _, topic := range c.Topics {
		// if topic has no more clients, then remove it
		if len(h.topics[topic]) == 0 {
			h.log.Println("[DEBUG] topic has no more clients, deleting from hub.")
			delete(h.topics, topic)
		}
	}
}

// doEmit sends message m to all of the channels of the topic's connections.
func (h *Hub) doEmit(m PublishMessage) {
	defer h.Unlock()

	h.Lock()

	// get topic subscribers
	clients, ok := h.topics[m.Topic]
	if !ok {
		h.log.Println("[DEBUG] there are no subscriptions from:", m.Topic)
		return
	}

	h.log.Printf("[DEBUG] Sending message to topic %v. Connection count %d", m.Topic, len(clients))
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
