package hub

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

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

// upgrader is the websocket connection upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type clientServerWS struct {
	*websocket.Conn
	*Client
}

func (w *clientServerWS) SetClient(c *Client) {
	w.Client = c
}

func newClientServerWS(w http.ResponseWriter, r *http.Request, origins []string) (*clientServerWS, error) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return checkOrigin(r, origins) }
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Printf("[ERROR] failed to upgrade connection to websocket %v", err)
		return nil, err
	}

	return &clientServerWS{Conn: conn}, nil
}

// checkOrigin checks the requests origin
func checkOrigin(r *http.Request, allowedOrigins []string) bool {
	origin := r.Header["Origin"]
	if len(origin) == 0 {
		return true
	}
	u, err := url.Parse(origin[0])
	if err != nil {
		return false
	}
	var allow bool
	for _, o := range allowedOrigins {
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
		log.Printf(
			"[DEBUG] none of allowed origins: %s matched: %s\n",
			strings.Join(allowedOrigins, ", "),
			u.Host,
		)
	}
	return allow

}

// listenRead pumps messages from the websocket connection to the hub.
func (w *clientServerWS) listenRead() {
	defer func() {
		log.Println("[DEBUG] Calling unregister from listenRead")
		w.Client.hub.unregister <- w.Client
	}()
	w.SetReadLimit(maxMessageSize)
	if err := w.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		log.Println("[ERROR] failed to set socket read deadline:", err)
	}
	w.SetPongHandler(func(string) error {
		return w.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		// read message from ws sent by Client
		_, payload, err := w.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Println("[DEBUG] read message error. Client probably closed connection:", err)
			} else {
				log.Printf("[DEBUG] Unexpected error: %v", err)
			}
			break
		}

		actionMessage := &ActionMessage{}
		// message contains the topic to which user is subscribing to
		if err := json.Unmarshal(payload, actionMessage); err != nil {
			log.Printf(
				"[ERROR] invalid data sent for subscription:%v\n",
				actionMessage,
			)
			continue
		}

		switch action := actionMessage.Action; action {
		case "subscribe":
			log.Printf("[DEBUG] Client %s is subscribing. Removing all old subscriptions.", w.Client.ID)
			w.Client.ClearTopics()
			subMsg := &SubscriptionsMessage{}
			if err := json.Unmarshal(payload, subMsg); err != nil {
				log.Printf(
					"[ERROR] invalid data sent for subscription:%v\n",
					actionMessage,
				)
				continue
			}
			w.Client.SubscribeMultiple(subMsg.Topics)
		default:
			pubMsg := &PublishMessage{}
			if err := json.Unmarshal(payload, pubMsg); err != nil {
				log.Printf(
					"[ERROR] invalid data sent for subscription:%v\n",
					actionMessage,
				)
				continue
			}
			w.Client.hub.Publish(*pubMsg)
		}
	}

}

// listenWrite pumps messages from the hub to the websocket connection.
func (w *clientServerWS) listenWrite() {
	// write to connection
	ticker := time.NewTicker(pingPeriod)
	write := func(mt int, payload []byte) error {
		if err := w.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			return err
		}
		return w.WriteMessage(mt, payload)
	}

	// when function ends, close connection
	defer func() {
		ticker.Stop()
		w.Close()
	}()

	for {
		select {
		// listen for messages
		case message, ok := <-w.Client.send:
			if !ok {
				// ws was closed, so close on our end
				err := write(websocket.CloseMessage, []byte{})
				if err != nil {
					w.Client.hub.log.Println("[ERROR] socket already closed:", err)
				}
				return
			}
			// write to ws
			if err := write(websocket.TextMessage, message); err != nil {
				w.Client.hub.log.Println("[ERROR] failed to write socket message:", err)
				return
			}
		case <-ticker.C: // ping pong ws connection
			if err := write(websocket.PingMessage, []byte{}); err != nil {
				w.Client.hub.log.Println("[ERROR] failed to ping socket:", err)
				return
			}
		}
	}

}
