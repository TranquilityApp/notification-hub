// This is a message broker program featuring fan-out and one-to-one
// messages.

package hub

import (
	"os"
)

type Application struct {
	Hub
}

// Creates a new instance of the application,
// instantiatiing a Hub struct.
func NewApp() *Application {
	app := &Application{
		Hub: *NewHub(os.Stdout, "*"),
	}
	return app
}
