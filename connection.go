package hub

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var (
	// Time allowed to write a message to the peer.
	WriteWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	PongWait = 20 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	PingPeriod = (PongWait * 9) / 10
	// Maximum message size allowed from peer.
	MaxMessageSize int64 = 64 * 1024
)

type Subscription struct {
	AuthID     string
	Topic      string
	connection *connection
}

type subscriber struct {
	AuthID      string
	connections map[*connection]bool
	topics      map[string]bool
}

type connection struct {
	ws     *websocket.Conn
	send   chan []byte
	hub    *Hub
	closed bool
}

func (c *connection) close() {
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

// Reads message from the websocket connection to subscribe the user
func (c *connection) listenRead() {
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
		_, wsMessage, err := c.ws.ReadMessage()
		if err != nil {
			c.hub.log.Println("[DEBUG] read message error. Client probably closed connection:", err)
			break
		}

		message := &MailMessage{}
		// message contains the topic to which user is subscribing to
		if err := json.Unmarshal(wsMessage, message); err != nil {
			c.hub.log.Printf(
				"[ERROR] invalid data sent for subscription:%v\n",
				message,
			)
			continue
		}
		if message.Action == "subscribe" {
			// get the message embedded data
			connData := ConnMessage{}
			json.Unmarshal([]byte(message.Message), &connData)
			// message contains the username as Auth0ID
			// create the subscriptions
			s := &Subscription{
				AuthID:     connData.AuthID,
				Topic:      "BENotification",
				connection: c,
			}
			c.hub.subscribe <- s
			s = &Subscription{
				AuthID:     connData.AuthID,
				Topic:      "FLNotification",
				connection: c,
			}
			c.hub.subscribe <- s
			s = &Subscription{
				AuthID:     connData.AuthID,
				Topic:      "ECNotification",
				connection: c,
			}
			c.hub.subscribe <- s
			s = &Subscription{
				AuthID:     connData.AuthID,
				Topic:      "LC",
				connection: c,
			}
			c.hub.subscribe <- s
			// defined in notification API
			c.hub.InitSubscriberDataFunc(&connData)
		} else if message.Action == "publish" {
			topicSplit := strings.Split(message.Topic, ":")
			if topicSplit[1] == "LC" {
				c.hub.LCMessageFunc(message)
			} else {
				c.hub.Publish(message)
			}
		}
	}
}

// Listens to writes on to the connection
func (c *connection) listenWrite() {
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
			if err := write(websocket.PingMessage, []byte{}); err != nil {
				c.hub.log.Println("[ERROR] failed to ping socket:", err)
				return
			}
		}
	}
}
