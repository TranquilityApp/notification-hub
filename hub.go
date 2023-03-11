package hub

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

// Broker is the application structure
type Broker struct {
	Hub
}

type BrokerOption func(*Broker)

// Notifier is used to track hub channel actions. This is mainly used for testing in hub_test.go.
type Notifier interface {
	Notify(s string)
}

func WithNotifier(n Notifier) BrokerOption {
	return func(b *Broker) {
		b.notifier = n
	}
}

// NewBroker is a Broker constructor which instantiates the Hub.
func NewBroker(origins []string, opts ...BrokerOption) *Broker {
	broker := &Broker{
		Hub: *NewHub(os.Stdout, origins),
	}

	for _, opt := range opts {
		opt(broker)
	}

	return broker
}

// Hub contains the active subscribers with thread-safe connection management,
// handles subscriptions and broadcasts messages to the subscribers.
type Hub struct {
	// subscriberMessageBuffer controls the max number
	// of messages that can be queued for a subscriber
	// before it is kicked.
	//
	// Defaults to 16.
	subscriberMessageBuffer int

	// publishLimiter controls the rate limit applied to the publish endpoint.
	//
	// Defaults to one publish every 100ms with a burst of 8.
	publishLimiter *rate.Limiter

	// registered subscribers
	subscribersMu sync.Mutex
	subscribers   map[*Subscriber]bool

	// topics with its subscribers (subscribers)
	topics map[string][]*Subscriber

	// logger
	log *log.Logger

	// allowed origins for http requests.
	allowedOrigins []string

	notifier Notifier

	OnSubscribe func(*Subscription)
}

// NewHub Instantiates the Hub.
// Adds the factory design pattern as a websocket HTTP connection upgrader.
// Creates the broker's logger
func NewHub(logOutput io.Writer, origins []string) *Hub {
	h := &Hub{
		subscriberMessageBuffer: 16,
		allowedOrigins:          origins,
		subscribers:             make(map[*Subscriber]bool),
		topics:                  make(map[string][]*Subscriber),
		publishLimiter:          rate.NewLimiter(rate.Every(time.Millisecond*100), 8),
	}

	h.log = log.New(leveledLogWriter(logOutput), "", log.LstdFlags)

	return h
}

// ServeHTTP upgrades HTTP connection to a ws/wss connection.
// Sets up the connection and registers it to the hub for
// read/write operations.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.allowedOrigins,
	})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusInternalError, "")

	csws := &subscriberServerWS{Conn: c}
	subscriber := NewSubscriber(csws, h, uuid.New().String())
	csws.SetSubscriber(subscriber)

	err = h.registerSubscriber(r.Context(), subscriber)
	if errors.Is(err, context.Canceled) {
		return
	}
	if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
		websocket.CloseStatus(err) == websocket.StatusGoingAway {
		return
	}
	if err != nil {
		return
	}
}

// registerSubscriber registers the subscriber with the hub and begins reading and writing
func (h *Hub) registerSubscriber(ctx context.Context, s *Subscriber) error {
	h.addSubscriber(s)
	defer h.deleteSubscriber(s)

	go s.ws.listenWrite(ctx)

	if err := s.ws.listenRead(ctx); err != nil {
		return err
	}

	return nil
}

// deleteSubscriber removes Subscriber from hub topics and cleans topics up
func (h *Hub) deleteSubscriber(s *Subscriber) {
	h.subscribersMu.Lock()
	defer h.subscribersMu.Unlock()

	h.Notify("unregister")

	// get the subscriber
	_, ok := h.subscribers[s]
	if !ok {
		h.log.Println("[WARN] cannot unregister connection, it is not registered.")
		return
	}

	h.deleteTopicsFromSubscriber(s)
	h.handleEmptyTopics()

	delete(h.subscribers, s)
}

// subscriber adds the topic to the connection's slice of topics.
func (h *Hub) subscribe(s *Subscription) {
	h.subscribersMu.Lock()
	defer h.subscribersMu.Unlock()

	h.Notify("subscribe")

	// initialize topic if it doesn't exist yet.
	if _, ok := h.topics[s.Topic]; !ok {
		h.topics[s.Topic] = make([]*Subscriber, 0)
	}

	// add Subscriber to the hub topic's Subscribers.
	h.topics[s.Topic] = append(h.topics[s.Topic], s.Subscriber)

	h.log.Printf("[DEBUG] Subscriber %s subscribed to topic %s", s.Subscriber.ID, s.Topic)
	if h.OnSubscribe != nil {
		h.OnSubscribe(s)
	}
}

// adds the subscribers to the hub
func (h *Hub) addSubscriber(s *Subscriber) {
	h.subscribersMu.Lock()
	h.subscribers[s] = true
	h.subscribersMu.Unlock()
	h.Notify("register")
}

// deleteTopicsFromSubscriber removes the subscriber from topics in the hub.
func (h *Hub) deleteTopicsFromSubscriber(s *Subscriber) {
	// remove this subscriber from the topics
	for i := 0; i < len(s.Topics); i++ {
		topicSubscribers := h.topics[s.Topics[i]]
		idxOfTopicSubscriber := -1

		// find the index of the subscriber from the topic's subscriptions
		for idx, subscriber := range topicSubscribers {
			if subscriber == s {
				idxOfTopicSubscriber = idx
				break
			}
		}

		// use the found index to remove this subscriber from the topic's Subscribers
		if idxOfTopicSubscriber != -1 {
			h.log.Printf("[DEBUG] Removing subscriber %s from hub topic %s", s.ID, s.Topics[i])
			h.topics[s.Topics[i]] = removeTopicSubscriber(h.topics[s.Topics[i]], idxOfTopicSubscriber)
		}
	}
}

// deleteEmptyTopics is a garbage collecting function to remove any unused topics.
func (h *Hub) handleEmptyTopics() {
	for topic, subscribers := range h.topics {
		if len(subscribers) == 0 {
			h.log.Printf("[DEBUG] topic %s has no more subscribers, deleting from hub.", topic)
			delete(h.topics, topic)
		}
	}
}

// doEmit sends message m to all of the channels of the topic's connections.
func (h *Hub) doEmit(m PublishMessage) {
	h.subscribersMu.Lock()
	defer h.subscribersMu.Unlock()

	h.publishLimiter.Wait(context.Background())

	h.Notify("emit")

	subscribers, ok := h.topics[m.Topic]
	if !ok {
		h.log.Println("[DEBUG] there are no subscriptions from:", m.Topic)
		return
	}

	h.log.Printf("[DEBUG] Sending message to topic %v. Subscriber count %d", m.Topic, len(subscribers))
	for _, s := range subscribers {
		select {
		case s.msgs <- m.Payload:
		default:
			go s.closeSlow()
		}
	}
}

// Publish sends Mailmessage m to the hub's emitter.
func (h *Hub) Publish(m PublishMessage) {
	// notify each subscriber of a topic
	if len(m.Topic) > 0 {
		h.doEmit(m)
	}
}

func (h *Hub) Notify(s string) {
	if h.notifier != nil {
		h.notifier.Notify(s)
	}
}

func removeTopicSubscriber(currSubscribers []*Subscriber, index int) []*Subscriber {
	newSubs := make([]*Subscriber, 0)
	newSubs = append(newSubs, currSubscribers[:index]...)
	return append(newSubs, currSubscribers[index+1:]...)
}
