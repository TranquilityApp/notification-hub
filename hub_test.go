package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
	t.Run("Register a subscriber", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}
		mustRegister(broker, subscriber, t)
	})
}

func TestHub_DoUnregister(t *testing.T) {
	t.Run("Unregister a previously-registered subscriber", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
			hub:  &broker.Hub,
		}
		mustRegister(broker, subscriber, t)

		broker.Hub.doUnregister(subscriber)

		// hub should have no topics
		if len(broker.Hub.topics) != 0 {
			t.Fatalf("Incorrect number of topics, expected %d got %d", 0, len(broker.Hub.topics))
		}

		// hub should have no subscribers
		if len(broker.Hub.subscribers) != 0 {
			t.Fatalf("Incorrect number of subscribers, expected %d got %d", 0, len(broker.Hub.subscribers))
		}

		// subscriber.close should = true
		if subscriber == nil || !subscriber.closed {
			t.Fatal("Expected subscriber to by closed but closed is true")
		}
	})
}

func TestHub_deleteTopicSubscriber(t *testing.T) {
	t.Run("Delete a subscriber from a topic in the hub", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		mustRegister(broker, subscriber, t)

		s := &Subscription{
			Subscriber: subscriber,
			Topic:  "FAKETOPIC",
		}

		mustSubscribe(&broker.Hub, s, t)

		broker.Hub.deleteTopicSubscriber(subscriber)

		// topics should have "FAKETOPIC" with no subscribers
		subscribers, ok := broker.Hub.topics["FAKETOPIC"]
		if !ok {
			t.Fatalf("Hub should have topic %s", "FAKETOPIC")
		}

		found := false
		for _, c := range subscribers {
			if c == subscriber {
				found = true
				break
			}
		}

		if found {
			t.Fatalf("Subscriber should not be subscribed to topic %s", "FAKETOPIC")
		}

	})
}

func TestHub_handleEmptyTopics(t *testing.T) {
	t.Run("Delete a topic because it has no more subscribers", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		mustRegister(broker, subscriber, t)

		s := &Subscription{
			Subscriber: subscriber,
			Topic:  "FAKETOPIC",
		}

		// subscribe to topic
		mustSubscribe(&broker.Hub, s, t)

		// unsubscribe from topic
		broker.Hub.deleteTopicSubscriber(subscriber)

		// topic should still exist in hub at this point
		if len(broker.Hub.topics) != 1 {
			t.Fatalf("Broker hub has %d topics, expected %d", len(broker.Hub.topics), 1)
		}

		// remove topic from hub
		broker.Hub.handleEmptyTopics(subscriber)

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

		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		s := &Subscription{
			Subscriber: subscriber,
			Topic:  "FAKETOPIC",
		}

		mustSubscribe(&brokerServer.broker.Hub, s, t)

		mustEmit(brokerServer.broker, subscriber, t)
	})

	t.Run("Emit to topic that does not exist", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		msg := PublishMessage{
			Topic: "faketopic",
		}
		broker.Hub.doEmit(msg)
	})
}

func mustEmit(broker *Broker, subscriber *Subscriber, t *testing.T) {
	want := "payload"

	msg := PublishMessage{
		Topic:   "FAKETOPIC",
		Payload: []byte(want),
	}

	broker.Hub.doEmit(msg)

	got := getEmitMsg(subscriber.send)
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
		broker := NewBroker([]string{"*"})

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
	t.Run("Subscribe a subscriber to one topic", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		s := &Subscription{
			Subscriber: subscriber,
			Topic:  "FAKETOPIC",
		}

		mustSubscribe(&broker.Hub, s, t)
	})
}

func TestHub_DoSubscribeOverNetwork(t *testing.T) {
	t.Run("Start a server with 1 subscriber and subscribe to one topic", func(t *testing.T) {
		brokerServer := NewBrokerServer()
		server := httptest.NewServer(brokerServer)
		ws := mustDialWs(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")

		defer server.Close()
		defer ws.Close()

		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		s := &Subscription{
			Subscriber: subscriber,
			Topic:  "FAKETOPIC",
		}

		mustSubscribe(&brokerServer.broker.Hub, s, t)
	})
}

func TestHub_GetSubscriber(t *testing.T) {
	t.Run("Get subscriber in hub", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
		}

		mustRegister(broker, subscriber, t)

		c, ok := broker.Hub.getSubscriber(subscriber.ID)
		if !ok {
			t.Fatal("Unable to get subscriber")
		} else if c.ID != subscriber.ID {
			t.Fatalf("Expected %s, got %s", c.ID, subscriber.ID)
		}

	})
}

// NotificationSpy is used to track the channel calls from the hub
type NotificationSpy struct {
	Calls []string
	wg    *sync.WaitGroup
}

// Notify adds a call to NotificationSpy and decrements waitgroup count.
func (s *NotificationSpy) Notify(notification string) {
	s.Calls = append(s.Calls, notification)
	if s.wg != nil {
		s.wg.Done()
	}
}

func TestHub_Run(t *testing.T) {
	t.Run("All channels waiting", func(t *testing.T) {
		wg := &sync.WaitGroup{}
		wg.Add(4)

		spyNotifyPrinter := &NotificationSpy{wg: wg}

		broker := NewBroker(
			[]string{"*"},
			WithNotifier(spyNotifyPrinter),
		)

		registerChan := make(chan *Subscriber, 1)
		unregisterChan := make(chan *Subscriber, 1)
		subscribeChan := make(chan *Subscription, 1)
		emitChan := make(chan PublishMessage, 1)
		broker.register = registerChan
		broker.unregister = unregisterChan
		broker.subscribe = subscribeChan
		broker.emit = emitChan

		go broker.Run()

		subscriber := &Subscriber{
			ID:   "FAKEUSER|IDWEOW",
			send: make(chan []byte, 256),
			hub:  &broker.Hub,
		}

		s := &Subscription{
			Subscriber: subscriber,
			Topic:  "topic",
		}

		msg := PublishMessage{Topic: "topic"}

		go func() {
			broker.register <- subscriber
			broker.subscribe <- s
			broker.emit <- msg
			broker.unregister <- subscriber
		}()

		wg.Wait()

		want := []string{
			"register",
			"subscribe",
			"publish",
			"unregister",
		}

		if len(want) != len(spyNotifyPrinter.Calls) {
			t.Fatalf("Wanted calls %v got %v", want, spyNotifyPrinter.Calls)
		}
	})
}

func mustRegister(broker *Broker, subscriber *Subscriber, t *testing.T) {
	broker.doRegister(subscriber)

	if ok := broker.Hub.subscribers[subscriber]; !ok {
		t.Fatal("Subscriber did not get registered with the hub")
	}
}

func mustSubscribe(hub *Hub, s *Subscription, t *testing.T) {
	hub.doSubscribe(s)

	subscribers, ok := hub.topics[s.Topic]
	if !ok {
		t.Fatalf("Broker did not subscribe to topic %s", s.Topic)
	}

	foundSubscriber := false
	for _, c := range subscribers {
		if c == s.Subscriber {
			foundSubscriber = true
		}
	}

	if !foundSubscriber {
		t.Fatalf("Cannot find subscriber %v", s.Subscriber)
	}

	if !containsString(s.Topic, s.Subscriber.Topics) {
		t.Fatalf("Subscriber is not subscribed to topic %s", s.Topic)
	}

}

type BrokerServer struct {
	broker *Broker
	http.Handler
}

func NewBrokerServer() *BrokerServer {
	server := new(BrokerServer)
	broker := NewBroker([]string{"*"})

	go broker.Run()

	server.broker = broker

	router := mux.NewRouter()
	router.Handle("/ws", negroni.New(
		negroni.Wrap(broker),
	))

	server.Handler = router

	return server
}

func mustDialWs(t *testing.T, url string) *websocket.Conn {
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("could not open a ws connection on %s %v", url, err)
	}

	return ws
}
