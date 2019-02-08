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
	Ready	bool
	Firing	bool
	When	time.Time

	igniter	Igniter
}

type Igniter struct {
	TestPin 	gpio.PinIO
	FirePin		gpio.PinIO
}
var igniter *Igniter




func IgniterControl(w http.ResponseWriter, r *http.Request) {
	var pulse = 0 * time.Nanosecond;

	if r.Method == "POST" {
		for (igniter.TestPin.Read() == gpio.Low && pulse < 1 * time.Second) {
			pulse += 250 * time.Millisecond;

			igniter.FirePin.Out(gpio.Low)

			igniter.FirePin.Out(gpio.High)
			time.Sleep(pulse)
			igniter.FirePin.Out(gpio.Low)
		}

		if (pulse.Nanoseconds() == 0){
			w.WriteHeader(http.StatusConflict)
		} else if (pulse.Seconds() >= 1) {
			w.WriteHeader(http.StatusExpectationFailed)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}

	istate := IgniterState {
		(igniter.TestPin.Read() == gpio.Low),
		(igniter.FirePin.Read() == gpio.High),
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

// TODO: HX711 IIO Scale Driver Interactions.

// TODO: Camera Interactions.

func main() {
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	igniter = &Igniter {
		TestPin: gpioreg.ByName("GPIO17"),
		FirePin: gpioreg.ByName("GPIO27"),
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r * http.Request) {
		fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
	})

	http.HandleFunc("/igniter", IgniterControl)



	log.Fatal(http.ListenAndServe(":80", nil));
}
