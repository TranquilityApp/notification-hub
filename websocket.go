package hub

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"nhooyr.io/websocket"
)

type subscriberServerWS struct {
	*websocket.Conn
	*Subscriber
}

func (w *subscriberServerWS) SetSubscriber(s *Subscriber) {
	w.Subscriber = s
}

// listenRead pumps messages from the websocket connection to the hub.
func (w *subscriberServerWS) listenRead(ctx context.Context) error {
	for {
		_, b, err := w.Conn.Read(ctx)
		if err != nil {
			w.hub.log.Println("[ERROR] get reader:", err)
			return err
		}

		actionMessage := &ActionMessage{}

		// message contains the topic to which user is subscribing to
		if err := json.Unmarshal(b, actionMessage); err != nil {
			log.Printf("[ERROR] invalid data sent for subscription:%v\n", actionMessage)
			return err
		}

		switch action := actionMessage.Action; action {
		case "subscribe":
			log.Printf("[DEBUG] Subscriber %s is subscribing. Removing all old subscriptions.", w.Subscriber.ID)
			w.Subscriber.ClearTopics()
			subMsg := &SubscriptionsMessage{}
			if err := json.Unmarshal(b, subMsg); err != nil {
				log.Printf("[ERROR] invalid data sent for subscription:%v\n", actionMessage)
				return err
			}
			w.Subscriber.SubscribeMultiple(subMsg.Topics)
		default:
			pubMsg := &PublishMessage{}
			if err := json.Unmarshal(b, pubMsg); err != nil {
				log.Printf("[ERROR] invalid data sent for subscription:%v\n", actionMessage)
				return err
			}
			w.Subscriber.hub.Publish(*pubMsg)
		}
	}
}

// listenWrite pumps messages from the hub to the websocket connection.
// this function is called when there is an emit.
func (w *subscriberServerWS) listenWrite(ctx context.Context) error {
	for {
		msg := <-w.Subscriber.msgs
		err := writeTimeout(ctx, time.Second*5, w.Conn, msg)
		if err != nil {
			return err
		}
	}
}

func writeTimeout(ctx context.Context, timeout time.Duration, c *websocket.Conn, msg []byte) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.Write(ctx, websocket.MessageText, msg)
}
