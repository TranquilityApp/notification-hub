package hub

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var (
	// WriteWait is the time allowed to write a message to the peer.
	WriteWait = 10 * time.Second
	// PongWait is the time allowed to read the next pong message from the peer.
	PongWait = 20 * time.Second
	// PingPeriod send pings to peer with this period. Must be less than pongWait.
	PingPeriod = (PongWait * 9) / 10
	// MaxMessageSize is the maximum message size allowed from peer.
	MaxMessageSize int64 = 64 * 1024
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

func (c *Client) SubscribeMultiple(topics []string) {
	for _, topic := range topics {
		c.Subscribe(topic)
	}
}

// Unsubscribe will end the subscription from the topic.
func (c *Client) Unsubscribe(topic string) {
	idx := -1
	for i := 0; i < len(c.Topics); i++ {
		if c.Topics[i] == topic {
			idx = i
			break
		}
	}

	if idx != -1 {
		c.Topics = append(c.Topics[:idx], c.Topics[idx+1:]...)
	}
	c.hub.log.Printf("[INFO] Client %s unsubscribed from topic %s", c.ID, topic)
}

// close closes the connection's websocket.
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

// listenRead is the websocket listener and handles receiving messages along the wire
// from the client. Different actions can be described within the message to determine
// the next action to take.
// This function also determines the max message size, is the pong handler,
// and sends to the hub's unregister channel when the function ends, which
// occurs when the listener errors (connection closed).
func (c *Client) listenRead() {
	// when function completes, unregister this connection
	// and close it
	defer func() {
		c.hub.log.Println("[DEBUG] Calling unregister from listenRead")
		c.hub.unregister <- c
	}()
	c.ws.SetReadLimit(MaxMessageSize)
	if err := c.ws.SetReadDeadline(time.Now().Add(PongWait)); err != nil {
		c.hub.log.Println("[ERROR] failed to set socket read deadline:", err)
	}
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(PongWait))
	})
	for {
		// read message from ws sent by client
		_, payload, err := c.ws.ReadMessage()
		if err != nil {
			c.hub.log.Println("[DEBUG] read message error. Client probably closed connection:", err)
			break
		}

		message := &Message{}
		// message contains the topic to which user is subscribing to
		if err := json.Unmarshal(payload, message); err != nil {
			c.hub.log.Printf(
				"[ERROR] invalid data sent for subscription:%v\n",
				message,
			)
			continue
		}

		switch action := message.Action; action {
		case "subscribe":
			c.Subscribe(message.Topic)
		case "subscribe-multiple":
			subMsg := &SubscriptionsMessage{}
			if err := json.Unmarshal(payload, subMsg); err != nil {
				c.hub.log.Printf(
					"[ERROR] invalid data sent for subscription:%v\n",
					message,
				)
				continue
			}
			c.SubscribeMultiple(subMsg.Topics)
		case "unsubscribe":
			c.Unsubscribe(message.Topic)
		default:
			c.hub.log.Printf("Message action %v not supported", action)
		}
	}
}

// listenWrite is the websocket writer and handles the write deadlines,
// ping tick iterval, and listens on the connection's send channel for messages
// to be written over the wire to the websocket client (browser).
// Messages are received on the connection's send channel after the message
// is received on the channel, it is written to the websocket.
func (c *Client) listenWrite() {
	// write to connection
	write := func(mt int, payload []byte) error {
		if err := c.ws.SetWriteDeadline(time.Now().Add(WriteWait)); err != nil {
			return err
		}
		return c.ws.WriteMessage(mt, payload)
	}
	ticker := time.NewTicker(PingPeriod)

	// when function ends, close connection
	defer func() {
		ticker.Stop()
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
			if err := write(websocket.PingMessage, []byte{'p', 'i', 'n', 'g'}); err != nil {
				c.hub.log.Println("[ERROR] failed to ping socket:", err)
				return
			}
		}
	}
}
