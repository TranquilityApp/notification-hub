package hub

import (
	"log"
	"sync"
	"testing"
)

func mustAddTopic(client *Client, t *testing.T, topic string) {
	client.AddTopic(topic)
	if len(client.Topics) == 0 {
		t.Fatalf("Unable to add topic %s to client", topic)
	}
}

func TestClient_AddTopic(t *testing.T) {
	t.Run("Add topic to client", func(t *testing.T) {
		client := &Client{}
		topic := "faketopic"
		mustAddTopic(client, t, topic)
	})
}

func TestClient_ClearTopics(t *testing.T) {
	t.Run("Clear topics for a client", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
			hub:  &broker.Hub,
		}
		mustRegister(broker, client, t)
		topic := "fakeTopic"
		mustAddTopic(client, t, topic)
		client.ClearTopics()
		if len(client.Topics) > 0 {
			t.Fatal("Failed to clearTopics on client")
		}
	})
}

func TestClient_Subscribe(t *testing.T) {
	t.Run("Subscribe client to hub", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
			hub:  &broker.Hub,
		}
		mustRegister(broker, client, t)
		topic := "fakeTopic"

		var got *Subscription
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			got = <-broker.Hub.subscribe
			wg.Done()
		}()

		client.Subscribe(topic)
		wg.Wait()

		if got.Topic != topic {
			t.Fatalf("Got %s expected %s", got.Topic, topic)
		}
	})
}

func TestClient_SubscribeMultiple(t *testing.T) {
	t.Run("Subscribe to multiple topics", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
			hub:  &broker.Hub,
		}
		mustRegister(broker, client, t)

		topics := []string{"topic1", "topic2"}

		var wg sync.WaitGroup
		wg.Add(2)

		got := make([]*Subscription, 0)

		go func() {
			for {
				if len(got) == 2 {
					break
				}
				subscription := <-broker.Hub.subscribe
				got = append(got, subscription)
				wg.Done()
			}
		}()

		client.SubscribeMultiple(topics)
		wg.Wait()

		log.Println(got[0])

		if got[0].Topic != topics[0] {
			t.Fatalf("Got %s expected %s", got[0].Topic, topics[0])
		} else if got[1].Topic != topics[1] {
			t.Fatalf("Got %s expected %s", got[1].Topic, topics[1])
		}

	})
}

func TestClient_Close(t *testing.T) {
	t.Run("Close client", func(t *testing.T) {
		broker := NewBroker()
		client := &Client{
			closed: false,
			hub:    &broker.Hub,
		}

		client.close()
		want := true
		got := client.closed
		if got != want {
			t.Fatalf("Got %t expected %t", got, want)
		}
	})
}
