package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		broker := NewBroker()

		server := httptest.NewServer(http.HandlerFunc(broker.ServeHTTP))
		defer server.Close()

		_ := mustDialWs(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")
	})
}

func TestHub_DoSubscribe(t *testing.T) {
	t.Run("Start a server with 1 client and subscribe to two topics", func(t *testing.T) {
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

	})
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
