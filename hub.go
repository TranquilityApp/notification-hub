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

type SubscriberInitializer interface {
	InitSubscriberData(connMap map[string]interface{})
}

var (
	ReadBufferSize  int = 1024
	WriteBufferSize int = 1024
)

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
	sync.Mutex                                // protects connections
	connections map[*connection]*Subscription // simple to close or remove connections
	topics      map[string][]*connection      // key is topic, value is subscribers

	log            *log.Logger
	allowedOrigins []string
	wsConnFactory  websocket.Upgrader // websocket connection upgrader
	register       chan *connection   // register new connection on this channel
	unregister     chan *connection   // unregister connection channel
	subscribe      chan *Subscription // subscribe as user

	Mailbox               chan MailMessage      // fan out message to subscriber
	SubscriberInitializer SubscriberInitializer // defined in API to setup intial data
}

// InitSubscriberData is used to call the implementation of delegate
// SubscriberInitializer.
func InitSubscriberData(s SubscriberInitializer, msgMap map[string]interface{}) {
	s.InitSubscriberData(msgMap)
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
		Mailbox:        make(chan MailMessage),
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

// Connections gets the count of hub's connections.
func (h *Hub) Connections() int {
	h.Lock()
	defer h.Unlock()
	return len(h.connections)
}

// doRegister prepares the Hub for the connection
func (h *Hub) doRegister(c *connection) {
	h.Lock()
	defer h.Unlock()
	h.connections[c] = nil
}

// doMailbox sends message m to all of the channels of the topic's connections.
func (h *Hub) doMailbox(m MailMessage) {
	defer h.Unlock()

	h.Lock()

	// get all connections (subscribers) of a topic.
	connections, ok := h.topics[m.Topic]
	if !ok {
		h.log.Println("[DEBUG] there are no subscriptions from:", m.Topic)
		return
	}

	// encode message.
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

// doSubscribe adds the topic to the connection's slice of topics.
func (h *Hub) doSubscribe(s *Subscription) {
	h.Lock()
	defer h.Unlock()

	s.connection.Topics = append(s.connection.Topics, s.Topic)

	// initalize the topic's slice of connection pointers if
	// the topic doesn't exist in the hub yet.
	if _, ok := h.topics[s.Topic]; !ok {
		h.topics[s.Topic] = make([]*connection, 0)
	}

	// add connection to the hub topic's connections.
	h.topics[s.Topic] = append(h.topics[s.Topic], s.connection) // only add topic to hub if it isn't already there

	// set the connection's subscriber
	h.connections[s.connection] = s

	h.log.Printf("[DEBUG] subscribed as %s to topic %s\n", s.AuthID, s.Topic)
}

// doUnregister unregisters a connection from the hub.
func (h *Hub) doUnregister(c *connection) {
	h.Lock()
	defer h.Unlock()

	h.log.Println("[DEBUG] Unregistering connection.")

	// get the subscriber
	_, ok := h.connections[c]
	if !ok {
		h.log.Println("[WARN] cannot unregister connection, it is not registered.")
		return
	}

	h.tidyTopics(c)
	c.close()
	delete(h.connections, c)
}

// tidyTopics removes the connection from the topic in the hub.
// and removes the topic from the hub if that topic has no more
// connections.
func (h *Hub) tidyTopics(c *connection) {
	h.log.Println("[DEBUG] cleaning up topics")
	// remove each connection from the hub's topic connections
	for i := 0; i < len(c.Topics); i++ {
		// remove connection from topics
		conns := h.topics[c.Topics[i]]
		foundIdx := -1
		// find the index of this connection in the list of connections that are subscribed to this topic
		for idx, conn := range conns {
			if conn == c {
				foundIdx = idx
				break
			}
		}

		// use the found index to remove this connection from the topic's connections
		if foundIdx != -1 {
			h.topics[c.Topics[i]] = append(conns[:foundIdx], conns[foundIdx+1:]...)
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
}

// doUnsubscribeTopics unsubscribes all topics from a connection.
func (h *Hub) doUnsubscribeTopics(c *connection) {
	h.Lock()
	defer h.Unlock()

	_, ok := h.connections[c]
	if !ok {
		h.log.Println("[WARN] cannot unsubscribe from topics, connection is not registered.")
		return
	}

	h.tidyTopics(c)

	// reset this connection's topics
	c.Topics = make([]string, 0)
}

// checkOrigin check's and validates the request's Origin header.
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

// Publish sends Mailmessage m to the hub's Mailbox.
func (h *Hub) Publish(m MailMessage) {
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
