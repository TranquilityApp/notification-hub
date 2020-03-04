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

type initSubscriberDataFunc func(m *ConnMessage)
type lcMessageFunc func(m *MailMessage)

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

	InitSubscriberDataFunc initSubscriberDataFunc
	LCMessageFunc          lcMessageFunc
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

func (h *Hub) Connections() int {
	h.Lock()
	defer h.Unlock()
	return len(h.connections)
}

// Prepares the Hub for the connection by setting it to nil
func (h *Hub) doRegister(c *connection) {
	h.Lock()
	defer h.Unlock()
	h.connections[c] = nil
}

// sends a message to all connections a subscriber has
func (h *Hub) doMailbox(m *MailMessage) {
	h.Lock()
	defer h.Unlock()

	sArray := strings.Split(m.Topic, ":")
	authID := sArray[0]

	s, ok := h.subscribers[authID]
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

// Occurs after the subscription has been created in the listenRead
// method of the connection. This function checks for an existing
// subscriber of the topic the subscriber wishes to subscribe to, and if it
// doesn't exist, establishes it as a new subscription (subscription has multiple conns)
func (h *Hub) doSubscribe(s *Subscription) {
	h.Lock()
	defer h.Unlock()

	// check if there already is a subscriber, if not then create the subscription
	newSubscriber, alreadyAvailable := h.subscribers[s.AuthID]
	if !alreadyAvailable {
		// subscriber doesn't exist yet, create a new one
		newSubscriber = &subscriber{
			AuthID:      s.AuthID,
			connections: make(map[*connection]bool),
			topics:      make(map[string]bool, 0),
		}
	}

	_, alreadyTaken := newSubscriber.topics[s.Topic]
	if !alreadyTaken {
		newSubscriber.topics[s.Topic] = true
	}

	newSubscriber.connections[s.connection] = true
	h.connections[s.connection] = newSubscriber
	h.subscribers[s.AuthID] = newSubscriber
	h.log.Printf("[DEBUG] subscribed as %s to topic %s\n", s.AuthID, s.Topic)
}

// Unregisters a connection from the hub
func (h *Hub) doUnregister(c *connection) {
	h.Lock()
	defer h.Unlock()

	// get the subscriber
	s, ok := h.connections[c]
	if !ok {
		h.log.Println(
			"[WARN] cannot unregister connection, it is not registered.",
		)
		return
	}

	if s != nil {
		h.log.Printf(
			"[DEBUG] unregistering one of subscribers: %s connections\n",
			s.AuthID,
		)
		// delete the connection from subscriber's connections
		delete(s.connections, c)
		if len(s.connections) == 0 {
			// there are no more open connections for this subscriber
			h.log.Printf(
				"[DEBUG] unsub: %s, no more open connections\n",
				s.AuthID,
			)
			delete(h.subscribers, s.AuthID)
		}
	}

	h.log.Println("[DEBUG] unregistering socket connection")
	c.close()
	delete(h.connections, c)
}

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
		h.log.Printf(
			"[DEBUG] none of allowed origins: %s matched: %s\n",
			strings.Join(h.allowedOrigins, ", "),
			u.Host,
		)
	}
	return allow
}

// Notifies the topic with a message
func (h *Hub) Publish(m *MailMessage) {
	// notify each subscriber of a topic
	if len(m.Topic) > 0 {
		h.Mailbox <- m
	}
	return
}

// Starts the Hub.
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
