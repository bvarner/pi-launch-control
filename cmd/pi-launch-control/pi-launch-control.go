package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"periph.io/x/periph/host"
	"time"

	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
)

type IgniterState struct {
	ready	bool
	firing	bool
	when	time.Time
	igniter	Igniter		`json:"-"`
}

type Igniter struct {
	testPin 	gpio.PinIO
	firePin		gpio.PinIO
}
var igniter *Igniter



func IgniterControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		if (igniter.firePin.Read() == gpio.Low) {
			// TODO: Fire until the test pin is high, or up to a 1 second pulse.
			igniter.firePin.Out(gpio.Low)

			igniter.firePin.Out(gpio.High)
			time.Sleep(250 * time.Millisecond)
			igniter.firePin.Out(gpio.Low)

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusConflict)
		}
	}

	istate := IgniterState {
		(igniter.testPin.Read() == gpio.Low),
		(igniter.firePin.Read() == gpio.High),
		time.Now(),
		*igniter,
	}

	if r.Method == "GET" || r.Method == "POST" {
		json.NewEncoder(w).Encode(istate)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"))
	}
}

func main() {
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	igniter = &Igniter {
		testPin: gpioreg.ByName("GPIO17"),
		firePin: gpioreg.ByName("GPIO27"),
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r * http.Request) {
		fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
	})

	http.HandleFunc("/igniter", IgniterControl)

	log.Fatal(http.ListenAndServe(":80", nil));
}
