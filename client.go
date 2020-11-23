package hub

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var (
	// WriteWait is the time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// PongWait is the time allowed to read the next pong message from the peer.
	pongWait = 30 * time.Second

	// PingPeriod send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// MaxMessageSize is the maximum message size allowed from peer.
	maxMessageSize int64 = 512
)

// Subscription represents a 1:1 relationship between topic and client.
type Subscription struct {
	Topic  string
	Client *Client
}

// Client represents a single connection from a user.
type Client struct {
	ID     string
	ws     *websocket.Conn
	hub    *Hub
	closed bool
	send   chan []byte
	Topics []string
}

// NewClient creates a new client.
func NewClient(ws *websocket.Conn, h *Hub) *Client {
	return &Client{
		ID:   uuid.New().String(),
		send: make(chan []byte, 256),
		ws:   ws,
		hub:  h,
	}
}

// AddTopic adds a topic to a client.
func (c *Client) AddTopic(topic string) {
	c.Topics = append(c.Topics, topic)
}

// Subscribe subscribes a client to a topic.
func (c *Client) Subscribe(topic string) {
	s := &Subscription{
		Topic:  topic,
		Client: c,
	}
	c.hub.subscribe <- s
}

// SubscribeMultiple subscribes the client to multiple topics.
func (c *Client) SubscribeMultiple(topics []string) {
	for _, topic := range topics {
		c.Subscribe(topic)
	}
}

// close closes the websocket and the send channel.
func (c *Client) close() {
	if !c.closed {
		if err := c.ws.Close(); err != nil {
			c.hub.log.Println("[DEBUG] websocket was already closed:", err)
		} else {
			c.hub.log.Println("[DEBUG] websocket closed.")
			c.hub.log.Println("[DEBUG] closing connection's send channel.")
			close(c.send)
		}
		c.closed = true
	}
}

// listenRead pumps messages from the websocket connection to the hub.
func (c *Client) listenRead() {
	// when function completes, unregister this connection
	// and close it
	defer func() {
		c.hub.log.Println("[DEBUG] Calling unregister from listenRead")
		c.hub.unregister <- c
	}()
	c.ws.SetReadLimit(maxMessageSize)
	if err := c.ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		c.hub.log.Println("[ERROR] failed to set socket read deadline:", err)
	}
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		// read message from ws sent by client
		_, payload, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.log.Println("[DEBUG] read message error. Client probably closed connection:", err)
			} else {
				c.hub.log.Println("[DEBUG] Unexpected error: %v", err)
			}
			break
		}

		actionMessage := &ActionMessage{}
		// message contains the topic to which user is subscribing to
		if err := json.Unmarshal(payload, actionMessage); err != nil {
			c.hub.log.Printf(
				"[ERROR] invalid data sent for subscription:%v\n",
				actionMessage,
			)
			continue
		}

		switch action := actionMessage.Action; action {
		case "subscribe":
			subMsg := &SubscriptionsMessage{}
			if err := json.Unmarshal(payload, subMsg); err != nil {
				c.hub.log.Printf(
					"[ERROR] invalid data sent for subscription:%v\n",
					actionMessage,
				)
				continue
			}
			c.SubscribeMultiple(subMsg.Topics)
		default:
			c.hub.log.Printf("Message action %v not supported", action)
		}
	}
}

// listenWrite pumps messages from the hub to the websocket connection.
func (c *Client) listenWrite() {
	// write to connection
	ticker := time.NewTicker(pingPeriod)
	write := func(mt int, payload []byte) error {
		if err := c.ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			return err
		}
		return c.ws.WriteMessage(mt, payload)
	}

	// when function ends, close connection
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()

	for {
		select {
		// listen for messages
		case message, ok := <-c.send:
			if !ok {
				// ws was closed, so close on our end
				err := write(websocket.CloseMessage, []byte{})
				if err != nil {
					c.hub.log.Println("[ERROR] socket already closed:", err)
				}
				return
			}
			// write to ws
			if err := write(websocket.TextMessage, message); err != nil {
				c.hub.log.Println("[ERROR] failed to write socket message:", err)
				return
			}
		case <-ticker.C: // ping pong ws connection
			if err := write(websocket.PingMessage, []byte{}); err != nil {
				c.hub.log.Println("[ERROR] failed to ping socket:", err)
				return
			}
		}
	}
}
