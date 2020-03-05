package hub

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	ReadBufferSize  int = 1024
	WriteBufferSize int = 1024
)

type Application struct {
	Hub
}

// Creates a new instance of the application,
// instantiatiing a Hub struct.
func NewApp() *Application {
	app := &Application{
		Hub: *NewHub(os.Stdout, "*"),
	}
	return app
}

type Hub struct {
	sync.Mutex                              // protects connections
	connections map[*connection]*subscriber // simple to close or remove connections
	subscribers map[string]*subscriber      // simple to find subscriber based on identifier string

	log            *log.Logger
	allowedOrigins []string
	wsConnFactory  websocket.Upgrader // websocket connection upgrader
	register       chan *connection   // register new connection on this channel
	unregister     chan *connection   // unregister connection channel
	subscribe      chan *Subscription // subscribe as user

	Mailbox chan *MailMessage // fan out message to subscriber
}

// Instantiates the Hub.
// Adds the factory design pattern as a websocket HTTP connection upgrader.
// Creates the application's logger
func NewHub(logOutput io.Writer, origins ...string) *Hub {
	h := &Hub{
		allowedOrigins: origins,
		register:       make(chan *connection),
		unregister:     make(chan *connection),
		connections:    make(map[*connection]*subscriber),
		subscribers:    make(map[string]*subscriber),
		subscribe:      make(chan *Subscription),
		Mailbox:        make(chan *MailMessage),
	}

	factory := websocket.Upgrader{
		ReadBufferSize:  ReadBufferSize,
		WriteBufferSize: WriteBufferSize,
		CheckOrigin:     h.checkOrigin,
	}

	h.wsConnFactory = factory

	if nil == logOutput {
		logOutput = ioutil.Discard
	}
	h.log = log.New(leveledLogWriter(logOutput), "", log.LstdFlags)

	return h
}

// Upgrades HTTP connections to ws/wss connections
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
	ws, err := h.wsConnFactory.Upgrade(w, r, nil)
	if err != nil {
		h.log.Println("[ERROR] failed to upgrade connection:", err)
		return
	}

	// create the connection
	c := &connection{send: make(chan []byte, 256), ws: ws, hub: h}

	// registers the connection to the hub by prepping the hub for the connection
	// by setting it to nil
	h.register <- c

	go c.listenWrite()
	c.listenRead()
}

// Connections retrieves the length of connections that the hub has.
func (h *Hub) Connections() int {
	h.Lock()
	defer h.Unlock()
	return len(h.connections)
}

// doRegister prepares the Hub for the connection by setting it to nil
func (h *Hub) doRegister(c *connection) {
	h.Lock()
	defer h.Unlock()
	h.connections[c] = nil
}

// doMailbox sends a message to all connections a subscriber has.
func (h *Hub) doMailbox(m *MailMessage) {
	h.Lock()
	defer h.Unlock()

	s, ok := h.subscribers[m.UserID]
	if !ok {
		h.log.Println("[DEBUG] there are no subscription from:", authID)
		return
	}

	bytes, err := json.Marshal(m)
	if err != nil {
		h.log.Println("[ERROR] Unable to marshal message")
	}

	h.log.Println("[DEBUG] subscriber connection count:", len(s.connections))
	for c := range s.connections {
		c.send <- bytes
	}
}

// doBroadcast sends a message to all connections in the hub.
func (h *Hub) doBroadcast(m *MailMessage) {
	h.Lock()
	defer h.Unlock()

	bytes, err := json.Marshal(m)
	if err != nil {
		h.log.Println("[ERROR] Unable to marshal message")
	}

	h.log.Println("[DEBUG] subscriber connection count:", len(s.connections))
	for c := range s.connections {
		c.send <- bytes
	}
}

// doSubscribe subscribes a user to a topic.
func (h *Hub) doSubscribe(s *Subscription) {
	h.Lock()
	defer h.Unlock()

	// check if this user is already a subscriber. If not then create the subscription
	newSubscriber, alreadyAvailable := h.subscribers[s.UserID]
	if !alreadyAvailable {
		// subscriber doesn't exist yet, create a new one
		newSubscriber = &subscriber{
			UserID:      s.UserID,
			connections: make(map[*connection]bool),
			topics:      make(map[string]bool, 0),
		}
	}

	newSubscriber.topics[s.Topic] = true           // subscribe to topic
	newSubscriber.connections[s.connection] = true // add the subscription's connection to the subscriber's connection pool
	h.connections[s.connection] = newSubscriber    // add the connection to the hub's pool of connections
	h.subscribers[s.UserID] = newSubscriber        // add the subscriber to the hub's pool of subscriptions

	h.log.Printf("[DEBUG] subscribed as %s to topic %s\n", s.UserID, s.Topic)
}

// doUnregister unregisters a connection from the hub.
func (h *Hub) doUnregister(c *connection) {
	h.Lock()
	defer h.Unlock()

	// get the subscriber
	s, ok := h.connections[c]
	if !ok {
		h.log.Println("[WARN] cannot unregister connection, it is not registered.")
		return
	}

	if s != nil {
		h.log.Printf("[DEBUG] unregistering one of subscribers: %s connections\n", s.UserID)
		// delete the connection from subscriber's connections
		delete(s.connections, c)
		if len(s.connections) == 0 {
			h.log.Printf("[DEBUG] unsub: %s, no more open connections\n", s.UserID)
			delete(h.subscribers, s.UserID)
		}
	}

	h.log.Println("[DEBUG] unregistering socket connection")
	c.close()
	delete(h.connections, c)
}

// checkOrigin looks at the request's Origin header for only allowable origins.
func (h *Hub) checkOrigin(r *http.Request) bool {
	origin := r.Header["Origin"]
	if len(origin) == 0 {
		return true
	}

	u, err := url.Parse(origin[0])
	if err != nil {
		return false
	}

	var allow bool
	for _, o := range h.allowedOrigins {
		if o == u.Host {
			allow = true
			break
		}
		if o == "*" {
			allow = true
			break
		}
	}

	if !allow {
		h.log.Printf("[DEBUG] none of allowed origins: %s matched: %s\n", strings.Join(h.allowedOrigins, ", "), u.Host)
	}

	return allow
}

// Publish notifies the topic with a MailMessage.
func (h *Hub) Publish(m *MailMessage) {
	if len(m.Topic) > 0 {
		h.Mailbox <- m
	}
	return
}

// Run starts the Hub.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.doRegister(c)
		case c := <-h.unregister:
			h.doUnregister(c)
		case m := <-h.Mailbox:
			h.doMailbox(m)
		case s := <-h.subscribe:
			h.doSubscribe(s)
		}
	}
}
