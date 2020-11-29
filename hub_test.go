package hub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TranquilityApp/middleware"
	"github.com/codegangsta/negroni"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

func serveHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			break
		}
		err = c.WriteMessage(mt, message)
		if err != nil {
			break
		}
	}
}

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

		mustRegister(client, t)

	})
}

func testHub_DoUnregister(t *testing.T) {
	t.Run("Unregister a previously-registered client", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		mustRegister(client, t)

		broker.Hub.doUnregister(client)
		// test handleRemoveClient
		// test client close
		// test delete h.clients
	})
}

func mustRegister(client *Client, t *testing.T) {
	broker.doRegister(client)

	if ok := broker.Hub.clients[client]; !ok {
		t.Fatal("Client did not get registered with the hub")
	}
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
		broker := NewBroker()
		server := httptest.NewServer(NewBrokerServer())
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

		mustSubscribe(&broker.Hub, s, t)
	})
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

func dosubToss() {
	/*
		broker := NewBroker()
		topics := []string{"topic1", "topic2"}
		server := httptest.NewServer(http.HandlerFunc(broker.ServeHTTP))
		ws := mustDialWs(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")

		defer server.Close()
		defer ws.Close()

		writeWSMessage(t, ws, topics[0])
		writeWSMessage(t, ws, topics[1])

		time.Sleep(10000)

		within(t, 10000, func() {
			if len(broker.Hub.clients) != 2 {
				t.Errorf("Got %d clients, want %d", len(broker.Hub.clients), 2)
			}
		})
	*/
}

func writeWSMessage(t *testing.T, conn *websocket.Conn, message string) {
	t.Helper()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
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

func within(t *testing.T, d time.Duration, assert func()) {
	t.Helper()

	done := make(chan struct{}, 1)

	go func() {
		assert()
		done <- struct{}{}
	}()

	select {
	case <-time.After(d):
		t.Error("timed out")
	case <-done:
	}
}

/*
func newClient(url string, hub *Hub) (c *connection) {
	// Convert http://127.0.0.1 to ws://127.0.0.1
	u := "ws" + strings.TrimPrefix(url, "http")

	// Connect to the server
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		return nil
	}

	c = &connec{send: make(chan []byte, 256), ws: ws, hub: hub}
	client := NewClient(ws, hub, "auth0|FAKE1")
	return c
}
*/
/*
func TestHub_DoRegister(t *testing.T) {
	app := NewApp()
	s := httptest.NewServer(http.HandlerFunc(serveHTTP))
	defer s.Close()
	c := newClient(s.URL, &app.Hub)
	app.doRegister(c)
	_, ok := app.Hub.clients[c]
	if !ok {
		t.Fatal()
	}
	if len(app.Hub.clients) != 1 {
		t.Fatal()
	}
}
*/
/*
func TestHub_DoSubscribe(t *testing.T) {
	app := NewApp()

	s := httptest.NewServer(http.HandlerFunc(serveHTTP))
	defer s.Close()

	c := newClient(s.URL, &app.Hub)
	defer c.ws.Close()

	sub := &Subscription{
		Topic:  "someTopic",
		Client: c,
	}

	app.doSubscribe(sub)

	testCases := []struct {
		Subscription        *Subscription
		LenConnectionTopics int
		LenHubTopics        int
		LenHubConnections   int
		ExpectedErrors      bool
		Description         string
	}{
		{
			Subscription:        sub,
			LenConnectionTopics: 0,
			LenHubTopics:        1,
			LenHubClients:       1,
			ExpectedErrors:      true,
			Description:         "Invalid LenConnectionTopics",
		},
		{
			Subscription:        sub,
			LenConnectionTopics: 1,
			LenHubTopics:        1,
			LenHubClients:       1,
			ExpectedErrors:      false,
			Description:         "Valid LenConnectionTopics",
		},
		{
			Subscription:        sub,
			LenConnectionTopics: 1,
			LenHubTopics:        0,
			LenHubClients:       1,
			ExpectedErrors:      true,
			Description:         "Invalid LenHubTopics",
		},
		{
			Subscription:        sub,
			LenConnectionTopics: 1,
			LenHubTopics:        1,
			LenHubClients:       1,
			ExpectedErrors:      false,
			Description:         "Valid LenHubTopics",
		},
		{
			Subscription:        sub,
			LenConnectionTopics: 1,
			LenHubTopics:        1,
			LenHubClients:       0,
			ExpectedErrors:      true,
			Description:         "Invalid LenHubConnections",
		},
		{
			Subscription:        sub,
			LenConnectionTopics: 1,
			LenHubTopics:        1,
			LenHubClients:       1,
			ExpectedErrors:      false,
			Description:         "Valid LenHubConnections",
		},
		{
			Subscription:        &Subscription{},
			LenConnectionTopics: 1,
			LenHubTopics:        1,
			LenHubClients:       1,
			ExpectedErrors:      true,
			Description:         "Invalid subscription",
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf(testCase.Description), func(t *testing.T) {
			var hasErrors = false
			appConn := app.Hub.connections[testCase.Subscription.connection]
			if testCase.Subscription != appConn {
				hasErrors = true
			} else if testCase.LenConnectionTopics != len(sub.Client.Topics) {
				hasErrors = true
			} else if testCase.LenHubTopics != len(app.Hub.topics) {
				hasErrors = true
			} else if testCase.LenHubClients != len(app.Hub.clients) {
				hasErrors = true
			}
			if testCase.ExpectedErrors != hasErrors {
				t.Fatal()
			}
		})
	}

}
*/

/*
func TestHub_DoUnsubscribe(t *testing.T) {
	app := NewApp()
	s := httptest.NewServer(http.HandlerFunc(serveHTTP))
	defer s.Close()
	c1 := newConnection(s.URL, &app.Hub)
	defer c1.ws.Close()
	c2 := newConnection(s.URL, &app.Hub)
	defer c2.ws.Close()

	sub1 := &Subscription{
		AuthID:     "auth0|000000",
		Topic:      "0000000",
		connection: c1,
	}
	sub2 := &Subscription{
		AuthID:     "auth0|000000",
		Topic:      "0000000",
		connection: c2,
	}
	app.doSubscribe(sub1)
	app.doSubscribe(sub2)
	app.doUnregister(c1)

	testCases := []struct {
		Topic                  string
		LenHubTopicConnections int
		LenHubTopic            int
		ExpectedErrors         bool
		Description            string
	}{
		{
			Topic:                  "0000000",
			LenHubTopicConnections: 2,
			LenHubTopic:            1,
			ExpectedErrors:         true,
			Description:            "Invalid number of connections for topic",
		},
		{
			Topic:                  "0000000",
			LenHubTopicConnections: 1,
			LenHubTopic:            1,
			ExpectedErrors:         false,
			Description:            "Valid number of connections for topic",
		},
		{
			Topic:                  "0000000",
			LenHubTopicConnections: 1,
			LenHubTopic:            0,
			ExpectedErrors:         true,
			Description:            "Invalid number of topics",
		},
		{
			Topic:                  "0000000",
			LenHubTopicConnections: 1,
			LenHubTopic:            1,
			ExpectedErrors:         false,
			Description:            "Valid number of topics",
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf(testCase.Description), func(t *testing.T) {
			var hasErrors = false
			if testCase.LenHubTopicConnections != len(app.Hub.topics[testCase.Topic]) {
				hasErrors = true
			} else if testCase.LenHubTopic != len(app.Hub.topics[testCase.Topic]) {
				hasErrors = true
			}
			if testCase.ExpectedErrors != hasErrors {
				t.Fatal()
			}
		})
	}

}
*/

/*
func TestHub_DoMailbox(t *testing.T) {
	app := NewApp()
	s := httptest.NewServer(http.HandlerFunc(serveHTTP))
	defer s.Close()
	c := newConnection(s.URL, &app.Hub)
	defer c.ws.Close()

	sub := &Subscription{
		AuthID:     "auth0|000000",
		Topic:      "0000000",
		connection: c,
	}
	app.doSubscribe(sub)

	m := MailMessage{
		Topic:   "0000000",
		Message: []byte("message"),
	}
	app.doMailbox(m)

	message, ok := <-c.send // check if message sent on channel
	if !ok {
		t.Fatal("Socket closed")
	}
	if len(message) == 0 {
		t.Fatal("Message didn't send")
	}
	return
}
*/

/*
func TestHub_Publish(t *testing.T) {
	app := NewApp()
	m := MailMessage{
		Message: []byte("message"),
	}
	app.Publish(m) // only returns if there was no topic
	return
}

*/
