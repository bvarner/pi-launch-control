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
	when	time.Time
	igniter	Igniter		`json:"-"`
}

type Igniter struct {
	testPin 	gpio.PinIO
	firePin		gpio.PinIO
}
var igniter *Igniter



func HandleIgniter(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		istate := IgniterState {
			(igniter.testPin.Read() == gpio.Low),
			time.Now(),
			*igniter,
		}
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

	http.HandleFunc("/igniter", HandleIgniter)

	log.Fatal(http.ListenAndServe(":80", nil));
}
