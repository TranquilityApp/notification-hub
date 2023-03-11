package hub

import (
	"log"
	"sync"
	"testing"
)

func mustAddTopic(subscriber *Subscriber, t *testing.T, topic string) {
	subscriber.AddTopic(topic)
	if len(subscriber.Topics) == 0 {
		t.Fatalf("Unable to add topic %s to subscriber", topic)
	}
}

func TestSubscriber_AddTopic(t *testing.T) {
	t.Run("Add topic to subscriber", func(t *testing.T) {
		subscriber := &Subscriber{}
		topic := "faketopic"
		mustAddTopic(subscriber, t, topic)
	})
}

func TestSubscriber_ClearTopics(t *testing.T) {
	t.Run("Clear topics for a subscriber", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
			hub:  &broker.Hub,
		}
		mustRegister(broker, subscriber, t)
		topic := "fakeTopic"
		mustAddTopic(subscriber, t, topic)
		subscriber.ClearTopics()
		if len(subscriber.Topics) > 0 {
			t.Fatal("Failed to clearTopics on subscriber")
		}
	})
}

func TestSubscriber_Subscribe(t *testing.T) {
	t.Run("Subscribe subscriber to hub", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
			hub:  &broker.Hub,
		}
		mustRegister(broker, subscriber, t)
		topic := "fakeTopic"

		var got *Subscription
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			got = <-broker.Hub.subscribe
			wg.Done()
		}()

		subscriber.Subscribe(topic)
		wg.Wait()

		if got.Topic != topic {
			t.Fatalf("Got %s expected %s", got.Topic, topic)
		}
	})
}

func TestSubscriber_SubscribeMultiple(t *testing.T) {
	t.Run("Subscribe to multiple topics", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			ID:   "FAKEUSER|ID",
			send: make(chan []byte, 256),
			hub:  &broker.Hub,
		}
		mustRegister(broker, subscriber, t)

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

		subscriber.SubscribeMultiple(topics)
		wg.Wait()

		log.Println(got[0])

		if got[0].Topic != topics[0] {
			t.Fatalf("Got %s expected %s", got[0].Topic, topics[0])
		} else if got[1].Topic != topics[1] {
			t.Fatalf("Got %s expected %s", got[1].Topic, topics[1])
		}

	})
}

func TestSubscriber_Close(t *testing.T) {
	t.Run("Close subscriber", func(t *testing.T) {
		broker := NewBroker([]string{"*"})
		subscriber := &Subscriber{
			closed: false,
			hub:    &broker.Hub,
		}

		subscriber.close()
		want := true
		got := subscriber.closed
		if got != want {
			t.Fatalf("Got %t expected %t", got, want)
		}
	})
}
