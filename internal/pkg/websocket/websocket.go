package websocket

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// upgrader is the websocket connection upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// checkOrigin checks the requests origin
func checkOrigin(r *http.Request, allowedOrigins []string) bool {
	origin := r.Header["Origin"]
	if len(origin) == 0 {
		return true
	}
	u, err := url.Parse(origin[0])
	if err != nil {
		return false
	}
	var allow bool
	for _, o := range allowedOrigins {
		if o == u.Host {
			allow = true
			break
		}
		if o == "*" {
			allow = true
			break
		}
	}
	if !allow {
		log.Printf(
			"[DEBUG] none of allowed origins: %s matched: %s\n",
			strings.Join(allowedOrigins, ", "),
			u.Host,
		)
	}
	return allow

}

// Upgrade upgrades a connection to websocket.
func Upgrade(w http.ResponseWriter, r *http.Request, origins []string) (*websocket.Conn, error) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return checkOrigin(r, origins) }
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
