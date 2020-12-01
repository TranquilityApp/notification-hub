package hub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/TranquilityApp/middleware"
	"github.com/codegangsta/negroni"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

func TestHub_ServeHTTP(t *testing.T) {
	t.Run("GET /ws returns 101", func(t *testing.T) {
		server := httptest.NewServer(NewBrokerServer())
		defer server.Close()
		_ = mustDialWs(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")
	})
}

func TestHub_DoRegister(t *testing.T) {
	t.Run("Register a client", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}
		mustRegister(broker, client, t)
	})
}

func TestHub_DoUnregister(t *testing.T) {
	t.Run("Unregister a previously-registered client", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
			hub:  &broker.Hub,
		}
		mustRegister(broker, client, t)

		broker.Hub.doUnregister(client)

		// hub should have no topics
		if len(broker.Hub.topics) != 0 {
			t.Fatalf("Incorrect number of topics, expected %d got %d", 0, len(broker.Hub.topics))
		}

		// hub should have no clients
		if len(broker.Hub.clients) != 0 {
			t.Fatalf("Incorrect number of clients, expected %d got %d", 0, len(broker.Hub.clients))
		}

		// client.close should = true
		if client == nil || !client.closed {
			t.Fatal("Expected client to by closed but closed is true")
		}
	})
}

func TestHub_deleteTopicClient(t *testing.T) {
	t.Run("Delete a client from a topic in the hub", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		mustRegister(broker, client, t)

		s := &Subscription{
			Client: client,
			Topic:  "FAKETOPIC",
		}

		mustSubscribe(&broker.Hub, s, t)

		broker.Hub.deleteTopicClient(client)

		// topics should have "FAKETOPIC" with no clients
		clients, ok := broker.Hub.topics["FAKETOPIC"]
		if !ok {
			t.Fatalf("Hub should have topic %s", "FAKETOPIC")
		}

		found := false
		for _, c := range clients {
			if c == client {
				found = true
				break
			}
		}

		if found {
			t.Fatalf("Client should not be subscribed to topic %s", "FAKETOPIC")
		}

	})
}

func TestHub_handleEmptyTopics(t *testing.T) {
	t.Run("Delete a topic because it has no more clients", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		mustRegister(broker, client, t)

		s := &Subscription{
			Client: client,
			Topic:  "FAKETOPIC",
		}

		// subscribe to topic
		mustSubscribe(&broker.Hub, s, t)

		// unsubscribe from topic
		broker.Hub.deleteTopicClient(client)

		// topic should still exist in hub at this point
		if len(broker.Hub.topics) != 1 {
			t.Fatalf("Broker hub has %d topics, expected %d", len(broker.Hub.topics), 1)
		}

		// remove topic from hub
		broker.Hub.handleEmptyTopics(client)

		if len(broker.Hub.topics) != 0 {
			t.Fatalf("Failed to remove topic %s from hub", s.Topic)
		}
	})
}

func TestHub_doEmit(t *testing.T) {
	t.Run("Emit topic from hub", func(t *testing.T) {
		brokerServer := NewBrokerServer()
		server := httptest.NewServer(brokerServer)
		ws := mustDialWs(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")

		defer server.Close()
		defer ws.Close()

		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		s := &Subscription{
			Client: client,
			Topic:  "FAKETOPIC",
		}

		mustSubscribe(&brokerServer.broker.Hub, s, t)

		mustEmit(brokerServer.broker, client, t)
	})
}

func mustEmit(broker *Broker, client *Client, t *testing.T) {
	want := "payload"

	msg := PublishMessage{
		Topic:   "FAKETOPIC",
		Payload: []byte(want),
	}

	broker.Hub.doEmit(msg)

	got := getEmitMsg(client.send)
	if got != want {
		t.Fatalf("Got %s want %s", got, want)
	}
}

func getEmitMsg(c <-chan []byte) string {
	receive := <-c
	return string(receive)
}

func TestHub_Publish(t *testing.T) {
	t.Run("Publish message to hub", func(t *testing.T) {
		broker := NewBroker()

		msg := PublishMessage{
			Topic:   "FAKETOPIC",
			Payload: []byte("payload"),
		}

		var got PublishMessage
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			got = <-broker.Hub.emit // write
			wg.Done()
		}()

		broker.Hub.Publish(msg)
		wg.Wait()

		if got.Topic != msg.Topic { // read
			t.Fatalf("Expected %s got %s", msg.Topic, got.Topic)
		}

	})
}

func TestHub_DoSubscribe(t *testing.T) {
	t.Run("Subscribe a client to one topic", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		s := &Subscription{
			Client: client,
			Topic:  "FAKETOPIC",
		}

		mustSubscribe(&broker.Hub, s, t)
	})
}

func TestHub_DoSubscribeOverNetwork(t *testing.T) {
	t.Run("Start a server with 1 client and subscribe to one topic", func(t *testing.T) {
		brokerServer := NewBrokerServer()
		server := httptest.NewServer(brokerServer)
		ws := mustDialWs(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")

		defer server.Close()
		defer ws.Close()

		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		s := &Subscription{
			Client: client,
			Topic:  "FAKETOPIC",
		}

		mustSubscribe(&brokerServer.broker.Hub, s, t)
	})
}

func TestHub_GetClient(t *testing.T) {
	t.Run("Get client in hub", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		mustRegister(broker, client, t)

		c, ok := broker.Hub.getClient(client.ID)
		if !ok {
			t.Fatal("Unable to get client")
		} else if c.ID != client.ID {
			t.Fatalf("Expected %s, got %s", c.ID, client.ID)
		}

	})
}

func mustRegister(broker *Broker, client *Client, t *testing.T) {
	broker.doRegister(client)

	if ok := broker.Hub.clients[client]; !ok {
		t.Fatal("Client did not get registered with the hub")
	}
}

func mustSubscribe(hub *Hub, s *Subscription, t *testing.T) {
	hub.doSubscribe(s)

	clients, ok := hub.topics[s.Topic]
	if !ok {
		t.Fatalf("Broker did not subscribe to topic %s", s.Topic)
	}

	foundClient := false
	for _, c := range clients {
		if c == s.Client {
			foundClient = true
		}
	}

	if !foundClient {
		t.Fatalf("Cannot find client %v", s.Client)
	}

	if !containsString(s.Topic, s.Client.Topics) {
		t.Fatalf("Client is not subscribed to topic %s", s.Topic)
	}

}

type BrokerServer struct {
	broker *Broker
	http.Handler
}

func NewBrokerServer() *BrokerServer {
	server := new(BrokerServer)
	broker := NewBroker()
	go broker.Run()

	server.broker = broker

	router := mux.NewRouter()
	router.Handle("/ws", negroni.New(
		negroni.HandlerFunc(addUserID),
		negroni.Wrap(broker),
	))

	server.Handler = router

	return server
}

// addUserID is a middleware to add the AuthID of the connecting user from the Authorization
// header.
func addUserID(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	authID := "FAKEUSER|ID"
	ctx := context.WithValue(r.Context(), middleware.AuthKey, authID)
	r = r.WithContext(ctx)
	next(w, r)
}

func writeWSMessage(t *testing.T, conn *websocket.Conn, message []byte) {
	t.Helper()
	if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
		t.Fatalf("Could not send message over ws connection %v", err)
	}
}

func mustDialWs(t *testing.T, url string) *websocket.Conn {
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("could not open a ws connection on %s %v", url, err)
	}

	return ws
}
