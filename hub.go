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

type initSubscriberDataFunc func(connMap map[string]interface{})
type lcDeleteMessageFunc func(m *MailMessage)

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
	sync.Mutex                                // protects connections
	connections map[*connection]*Subscription // simple to close or remove connections
	topics      map[string][]*connection      // key is topic, value is subscribers

	log            *log.Logger
	allowedOrigins []string
	wsConnFactory  websocket.Upgrader // websocket connection upgrader
	register       chan *connection   // register new connection on this channel
	unregister     chan *connection   // unregister connection channel
	subscribe      chan *Subscription // subscribe as user

	Mailbox chan *MailMessage // fan out message to subscriber

	InitSubscriberDataFunc initSubscriberDataFunc
	LCDeleteMessageFunc    lcDeleteMessageFunc
}

// Instantiates the Hub.
// Adds the factory design pattern as a websocket HTTP connection upgrader.
// Creates the application's logger
func NewHub(logOutput io.Writer, origins ...string) *Hub {
	h := &Hub{
		allowedOrigins: origins,
		register:       make(chan *connection),
		unregister:     make(chan *connection),
		connections:    make(map[*connection]*Subscription),
		subscribe:      make(chan *Subscription),
		Mailbox:        make(chan *MailMessage),
		topics:         make(map[string][]*connection),
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

	connections, ok := h.topics[m.Topic]
	if !ok {
		h.log.Println("[DEBUG] there are no subscriptions from:", m.Topic)
		return
	}

	bytes, err := json.Marshal(m)
	if err != nil {
		h.log.Println("[ERROR] Unable to marshal message")
	}

	h.log.Println("[DEBUG] sending message to topic: ", m.Topic)
	h.log.Println("[DEBUG] topic connection count:", len(connections))
	for _, c := range connections {
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

	s.connection.Topics = append(s.connection.Topics, s.Topic)

	if _, ok := h.topics[s.Topic]; !ok {
		h.topics[s.Topic] = make([]*connection, 0)
	}

	h.topics[s.Topic] = append(h.topics[s.Topic], s.connection) // only add topic to hub if it isn't already there

	h.connections[s.connection] = s
	h.log.Printf("[DEBUG] subscribed as %s to topic %s\n", s.AuthID, s.Topic)
}

// Unregisters a connection from the hub
func (h *Hub) doUnregister(c *connection) {
	h.Lock()
	defer h.Unlock()

	// get the subscriber
	_, ok := h.connections[c]
	if !ok {
		h.log.Println("[WARN] cannot unregister connection, it is not registered.")
		return
	}

	h.log.Println("[DEBUG] unregistering connection")

	// remove each topic in the connection
	for i := 0; i < len(c.Topics); i++ {
		// remove connection from topics
		connList := h.topics[c.Topics[i]]
		foundIdx := -1
		for idx, conn := range connList {
			if conn == c {
				foundIdx = idx
				break
			}
		}

		// remove connection from topic's connections
		if foundIdx != -1 {
			h.topics[c.Topics[i]] = append(connList[:foundIdx], connList[foundIdx+1:]...)
		}
	}

	// check if the topic needs to be deleted from the hub by cycling through this subscriber's
	// topics.
	for _, topic := range c.Topics {
		// delete from array of topic's subscribers
		if len(h.topics[topic]) == 0 {
			// topic had 1 last subscriber
			h.log.Println("[DEBUG] topic has no more connections, deleting from hub.")
			delete(h.topics, topic)
		}
	}

	h.log.Println("[DEBUG] closing connection's websocket and channel.")
	c.close()
	h.log.Println("[DEBUG] deleting connection from hub's connections.")
	delete(h.connections, c)
}

func (h *Hub) doUnsubscribeTopics(c *connection) {
	h.Lock()
	defer h.Unlock()

	_, ok := h.connections[c]
	if !ok {
		h.log.Println("[WARN] cannot unsubscribe from topics, connection is not registered.")
		return
	}

	// remove each topic in the connection
	for i := 0; i < len(c.Topics); i++ {
		// remove connection from topics
		connList := h.topics[c.Topics[i]]
		foundIdx := -1
		for idx, conn := range connList {
			if conn == c {
				foundIdx = idx
				break
			}
		}

		// remove connection from topic's connections
		if foundIdx != -1 {
			h.topics[c.Topics[i]] = append(connList[:foundIdx], connList[foundIdx+1:]...)
		}
	}

	// check if the topic needs to be deleted from the hub by cycling through this subscriber's
	// topics.
	for _, topic := range c.Topics {
		// delete from array of topic's subscribers
		if len(h.topics[topic]) == 0 {
			// topic had 1 last subscriber
			h.log.Println("[DEBUG] topic has no more connections, deleting from hub.")
			delete(h.topics, topic)
		}
	}

	// reset this connection's topics
	c.Topics = make([]string, 0)

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
