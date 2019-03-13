package pi_launch_control

import (
	"encoding/json"
	"fmt"
)

type Emitter struct {
	// A map where the key is a channel to send data too, and the bool means nothing.
	listeners map[string][]chan string
}

func (e *Emitter) AddListener(event string, ch chan string) {
	if e.listeners == nil {
		e.listeners = make(map[string][]chan string)
	}
	if _, ok := e.listeners[event]; ok {
		// Append
		e.listeners[event] = append(e.listeners[event], ch)
	} else {
		// Create newly allocated
		e.listeners[event] = []chan string{ch}
	}
}

func (e *Emitter) RemoveListener(event string, ch chan string) {
	if _, ok := e.listeners[event]; ok {
		for i := range e.listeners[event] {
			if e.listeners[event][i] == ch {
				e.listeners[event] = append(e.listeners[event][:i], e.listeners[event][i + 1:]...)
				break;
			}
		}
	}
}

func (e *Emitter) Emit(v interface{}) {
	b, err := json.Marshal(v)
	if err == nil {
		for event, handlers := range e.listeners {
			s := fmt.Sprintf("event: %s\ndata: %s\n", event, string(b))
			for i := range handlers {
				go func(handler chan string) {
					handler <- s
				}(handlers[i])
			}
		}
	} else {
		fmt.Println("error: ", err)
	}
}
