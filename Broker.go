// Implements an SSE broker for HTML5 Clients
package pi_launch_control

import (
	"fmt"
	"net/http"
	"time"
)

type Broker struct {
	// Map of clients. Keys = channels over which we can push direct to attached client.
	// Values are actually just meaningless booleans.
	clients map[chan string]bool

	// Channel into which new clients can be pushed.
	newClients chan chan string

	// Channel into which disconnected clients should be pushed.
	defunctClients chan chan string

	// Channel into which message are pushed to be broadcast out to attached clients.
	Outgoing chan string
}

func NewBroker() *Broker {
	return &Broker {
		make(map[chan string]bool),
		make(chan (chan string)),
		make(chan (chan string)),
		make(chan string),
	}
}

func (b *Broker) Start() {
	go func() {
		// Loops forever... Perhaps we should allow for a shutdown?
		for {
			select {
			case s := <-b.newClients:
				// A new client is attached and we want to start sending them Outgoing.
				b.clients[s] = true



			case s := <-b.defunctClients:
				delete(b.clients, s)
				close(s)
			case msg := <-b.Outgoing:
				// There is a new message to send.
				for s := range b.clients {
					s <- msg
				}
			}
		}
	}()
}

// This is the method that handles HTTP requests / client communication.
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Make sure we support Flushing.
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported: ", http.StatusInternalServerError)
		return
	}

	// Create a new channel, over which the broker can send this client message.
	messageChan := make(chan string)

	// Add it to the list of new clients.
	b.newClients <- messageChan

	notify := r.Context().Done()
	go func() {
		<- notify
		b.defunctClients <- messageChan
	}()

	// set the headers related to SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	f.Flush()

	// Don't close the connection, instead just loop....
	for {
		msg, open := <- messageChan
		if  !open {
			// if the messageChan was closed, just bail.
			break
		}

		// Wrtie to the responsewriter.
		fmt.Fprintf(w, "id: %d\n%s\n", time.Now().Unix(), msg)
		f.Flush()
	}
}