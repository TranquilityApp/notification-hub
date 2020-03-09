package main

import "testing"

func TestSubscribe(t *testing.T) {

	var app *hub.Application
	app := hub.NewApp()

	go app.Run()

	ws, err := app.wsConnFactory.Upgrade(w, r, nil)
	if err != nil {
		h.log.Println("[ERROR] failed to upgrade connection:", err)
		return
	}

	c := &connection{send: make(chan []byte, 256), ws: ws, hub: h}

	tables := []struct {
		authID     string
		connection *connection
		topic      string
	}{
		{"greg", c, "TJ"},
		{"craig", c, "FL"},
		{"joel", c, "BE"},
	}

	for _, table := range tables {
		s := &Subscription{
			AuthID:     table.authID,
			Topic:      table.topic,
			connection: table.connection,
		}
		c.hub.subscribe <- s
	}
}

func TestHub_Publish(t *testing.T) {

}
