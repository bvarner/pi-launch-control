package pi_launch_control

import (
	"encoding/json"
	"fmt"
)

type EmitterID interface {
	eventName()	string
}

type Emitter struct {
	listeners []chan string
	EmitterID
}

func (e *Emitter) AddListener(ch chan string) {
	if e.listeners == nil {
		e.listeners = make([]chan string, 0)
	}
	e.listeners = append(e.listeners, ch)
}

func (e *Emitter) RemoveListener(event string, ch chan string) {
	for i := range e.listeners {
		if e.listeners[i] == ch {
			e.listeners = append(e.listeners[:i], e.listeners[i + 1:]...)
			break;
		}
	}
}

func (e *Emitter) Emit(v interface{}) {
	b, err := json.Marshal(v)
	if err == nil {
		for _, handler := range e.listeners {
			s := fmt.Sprintf("event: %s\ndata: %s\n", e.eventName(), string(b))
			go func(handler chan string) {
				handler <- s
			}(handler)
		}
	} else {
		fmt.Println("error: ", err)
	}
}
