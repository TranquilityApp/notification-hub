package hub

import (
	"github.com/truescotian/pubsub/internal/pkg/websocket"

	"encoding/json"
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

// Hub is the backbone of the broker. This function contains thread-safe
// connection management by containing the connections, topics,
// and logger. Channels are used to handle subscriptions, registering,
// and unregistering.
type Hub struct {
	sync.Mutex                      // protects connections
	clients    map[string]*Client   // hub list of clients. Client ID is key
	topics     map[string][]*Client // key is topic, value is Clients

	log            *log.Logger
	allowedOrigins []string
	register       chan *Client       // register new Client
	unregister     chan *Client       // unregister Client
	subscribe      chan *Subscription // subscribe as user

	Mailbox chan Message // fan out message to subscriber
}

// NewHub Instantiates the Hub.
// Adds the factory design pattern as a websocket HTTP connection upgrader.
// Creates the application's logger
func NewHub(logOutput io.Writer, origins ...string) *Hub {
	h := &Hub{
		allowedOrigins: origins,
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		clients:        make(map[string]*Client),
		subscribe:      make(chan *Subscription),
		Mailbox:        make(chan Message),
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

	c := NewClient(ws, h)

	h.register <- c

	go c.listenWrite()
	c.listenRead()
}

// addClient adds a client to the hub using its ID as the key.
func (h *Hub) addClient(c *Client) {
	h.clients[c.ID] = c
}

// Clients gets the count of hub's connections.
func (h *Hub) Clients() int {
	h.Lock()
	defer h.Unlock()
	return len(h.clients)
}

// doRegister prepares the Hub for the connection
func (h *Hub) doRegister(c *Client) {
	h.Lock()
	defer h.Unlock()
	h.addClient(c)
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
	_, ok := h.clients[c.ID]
	if !ok {
		h.log.Println("[WARN] cannot unregister connection, it is not registered.")
		return
	}

	h.cleanup(c)
	c.close()
	delete(h.clients, c.ID)
}

// cleanHub removes the client from topics in the hub. If the topic has no more clients,
// then the topic is removed from the hub.
func (h *Hub) cleanup(c *Client) {
	h.log.Println("[DEBUG] cleaning up topics")
	// remove each connection from the hub's topic connections
	for i := 0; i < len(c.Topics); i++ {
		// remove connection from topics
		clients := h.topics[c.Topics[i]]
		foundIdx := -1
		// find the index of this connection in the list of connections that are subscribed to this topic
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

// doMailbox sends message m to all of the channels of the topic's connections.
func (h *Hub) doMailbox(m Message) {
	defer h.Unlock()

	h.Lock()

	// get all connections (subscribers) of a topic.
	clients, ok := h.topics[m.Topic]
	if !ok {
		h.log.Println("[DEBUG] there are no subscriptions from:", m.Topic)
		return
	}

	// encode message.
	bytes, err := json.Marshal(m)
	if err != nil {
		h.log.Println("[ERROR] Unable to marshal message")
	}

	h.log.Printf("[DEBUG] Sending message to topic %v. Connection count %d", m.Topic, len(clients))
	for _, c := range clients {
		c.send <- bytes
	}
}

// Publish sends Mailmessage m to the hub's Mailbox.
func (h *Hub) Publish(m Message) {
	// notify each subscriber of a topic
	if len(m.Topic) > 0 {
		h.Mailbox <- m
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
		case m := <-h.Mailbox: // Sends message to a mailbox
			h.doMailbox(m)
		case s := <-h.subscribe: // Subscribes a user to the hub
			h.doSubscribe(s)
		}
	}
}
