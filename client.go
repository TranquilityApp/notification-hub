package hub

// Subscription represents a 1:1 relationship between topic and client.
type Subscription struct {
	Topic  string
	Client *Client
}

// Client represents a single connection from a user.
type Client struct {
	ID     string
	ws     *clientServerWS
	hub    *Hub
	closed bool
	send   chan []byte
	Topics []string
}

// NewClient creates a new client.
func NewClient(ws *clientServerWS, h *Hub, ID string) *Client {
	return &Client{
		ID:   ID,
		send: make(chan []byte, 256),
		ws:   ws,
		hub:  h,
	}
}

// AddTopic adds a topic to a client.
func (c *Client) AddTopic(topic string) {
	c.Topics = append(c.Topics, topic)
}

// RemoveTopic removes topics from a client and the client from hub's topic map
func (c *Client) ClearTopics() {
	c.hub.Lock()
	defer c.hub.Unlock()
	c.hub.handleRemoveClient(c) // removes client from hub topics and cleans topics up
	c.Topics = nil
}

// Subscribe subscribes a client to a topic.
func (c *Client) Subscribe(topic string) {
	s := &Subscription{
		Topic:  topic,
		Client: c,
	}
	c.hub.subscribe <- s
}

// SubscribeMultiple subscribes the client to multiple topics.
func (c *Client) SubscribeMultiple(topics []string) {
	for _, topic := range topics {
		c.Subscribe(topic)
	}
}

// close closes the websocket and the send channel.
func (c *Client) close() {
	if !c.closed {
		if c.ws == nil {
			c.hub.log.Println("[DEBUG] websocket was nil")
		} else if err := c.ws.Close(); err != nil {
			c.hub.log.Println("[DEBUG] websocket was already closed:", err)
		} else {
			c.hub.log.Println("[DEBUG] websocket closed.")
			c.hub.log.Println("[DEBUG] closing connection's send channel.")
			close(c.send)
		}
		c.closed = true
	}
}