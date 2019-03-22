package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/GeertJohan/go.rice"
	"github.com/bvarner/pi-launch-control"
	"log"
	"net/http"
	"os"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/host"
	"strconv"
	"time"
)

var testTrigger *time.Ticker

var igniter *pi_launch_control.Igniter

var scale *pi_launch_control.Scale

var camera *pi_launch_control.Camera

var broker *pi_launch_control.Broker

var handler http.Handler

func TestControl(w http.ResponseWriter, r *http.Request) {
	force := false
	keys, ok := r.URL.Query()["force"]
	if ok {
		force = len(keys) > 0
	}

	if (!scale.Initialized || !scale.Calibrated) && !force {
		w.Write([]byte("Scale Not Initialized or Not Calibrated."))
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	LaunchControl(w, r)
}

// TODO: Totally refactor this.
func LaunchControl(w http.ResponseWriter, r *http.Request) {
	if igniter.IsReady() {
		// Push the Camera and data feed file.
		p, ok := w.(http.Pusher)
		if ok {
			p.Push("/camera/video", nil)
			if scale.Initialized && scale.Calibrated {
				p.Push("/scale/capture", nil)
			}
			p.Push("/igniter/countdown", nil)
		}
	} else {
		w.Write([]byte("Igniter not ready."));
	}
	return
}

func ScaleSettingsControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var nscale *pi_launch_control.Scale
		json.NewDecoder(r.Body).Decode(nscale);

		// Alllows us to force a retry of New with the previous device settings if no JSON body is supplied.
		if nscale == nil {
			nscale = scale
		}

		nscale, err := pi_launch_control.NewScale(nscale.Device, nscale.TriggerC, nscale.Trigger);
		if err != nil {
			fmt.Println("Error updating scale.", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("500 - Internal Server Error"))
			return
		}
		scale = nscale
	}

	if r.Method == "GET" || r.Method == "POST" {
		json.NewEncoder(w).Encode(scale.Read())
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"));
	}
}


func TareScaleControl(w http.ResponseWriter, r *http.Request) {
	if scale.Initialized && (r.Method == "GET" || r.Method == "POST") {
		scale.Tare()
		json.NewEncoder(w).Encode(scale)
	} else if scale.Initialized {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"))
	} else {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404 - Scale Not Present"))
	}
}


func CalibrateScaleControl(w http.ResponseWriter, r *http.Request) {
	if scale.Initialized && (r.Method == "GET" || r.Method == "POST") {
		keys, ok := r.URL.Query()["mass"]
		if ok {
			mass, err := strconv.Atoi(keys[0])
			if err == nil {
				scale.Calibrate(mass)
				json.NewEncoder(w).Encode(scale)
			}
			return
		}
	} else if scale.Initialized {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"));
	} else {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404 - Scale Not Present"))
	}
}

func RootHandler(w http.ResponseWriter, r *http.Request) {
	// Push some things if we know what our request is.
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		p, ok := w.(http.Pusher)
		if ok {
			p.Push("/events", nil)
			p.Push("/style.css", nil)
			p.Push("/App.js", nil)
		}
	}

	handler.ServeHTTP(w, r)
}

func CameraStatusControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		json.NewEncoder(w).Encode(camera)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"))
	}
}

func IgniterControl(w http.ResponseWriter, r *http.Request) {
	var err error = nil
	if r.Method == "POST" {
		err = igniter.Fire(false);

		if err != nil {
			w.WriteHeader(http.StatusExpectationFailed)
			w.Write([]byte(err.Error() + "\n"))
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}

	if r.Method == "GET" || r.Method == "POST" {
		json.NewEncoder(w).Encode(igniter.GetState())
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"))
	}
}

func redirectTLS(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://" + r.Host + r.RequestURI, http.StatusMovedPermanently)
}


func main() {
	var err error = nil
	if _, err = host.Init(); err != nil {
		log.Fatal(err)
	}

	// Setup the Ticker for triggering scale and image capture.
	testTrigger := time.NewTicker(12500 * time.Microsecond) // 80hz
//	testTrigger := time.NewTicker(25000 * time.Microsecond) // 40hz
	// Create a channel for the scale and the camera.
	scaleTrigC := make(chan time.Time, 1)
	camTrigC   := make(chan time.Time, 1)
	// Go func to send to both of them when testTrigger is fired.
	go func() {
		for t := range testTrigger.C {
			scaleTrigC <- t
			camTrigC <- t
		}
	}()


	// Setup the SSE Broker
	broker = pi_launch_control.NewBroker()
	broker.Start()

	// Initialize the Igniter.
	igniter = &pi_launch_control.Igniter {
		TestPin: gpioreg.ByName("GPIO17"),
		FirePin: gpioreg.ByName("GPIO27"),
	}
	igniter.EmitterID = igniter
	igniter.AddListener(broker.Outgoing)

	// Initialize the Scale.
	scaleDevice := "/sys/devices/platform/0.weight"
	scaleTrigger := "/sys/bus/iio/devices/iio_sysfs_trigger/trigger0"
	scale, err = pi_launch_control.NewScale(scaleDevice, scaleTrigC, scaleTrigger);
	if err != nil {
		fmt.Println(err)
	} else {
		scale.AddListener(broker.Outgoing)
		fmt.Println("Scale Present and Tared");
		defer scale.Close()
	}

	// Initialize the Camera
	camera, err = pi_launch_control.NewCamera("/dev/video0", camTrigC)
	if err != nil {
		fmt.Println(err)
	} else {
		camera.AddListener(broker.Outgoing)
		fmt.Println("Camera Present and Initialized.")
		defer camera.Close()
	}

	fmt.Println("Setting up HTTP server...")

	handler = http.FileServer(rice.MustFindBox("webroot").HTTPBox())

	// Setup the handlers.
	http.HandleFunc("/", RootHandler)

	// Setup the SSE Event Handler. This comes from the 'broker'.
	http.HandleFunc("/events", broker.ServeHTTP)

	http.HandleFunc("/igniter", IgniterControl)

	http.HandleFunc("/camera", camera.ServeHTTP)
	http.HandleFunc("/camera/status", CameraStatusControl)

	http.HandleFunc("/scale", ScaleSettingsControl)
	http.HandleFunc("/scale/tare", TareScaleControl)
	http.HandleFunc("/scale/calibrate", CalibrateScaleControl)

	http.HandleFunc("/testfire", TestControl)
	http.HandleFunc("/launch", LaunchControl)

	cert := flag.String("cert", "/etc/ssl/certs/pi-launch-control.pem", "The certificate for this server.")
	certkey := flag.String("key", "/etc/ssl/certs/pi-launch-control-key.pem", "The key for the server cert.")

	flag.Parse()

	_, certerr := os.Stat(*cert)
	_, keyerr := os.Stat(*certkey)

	if certerr == nil && keyerr == nil {
		log.Println("SSL Configuration set up.")
		go func() {
			log.Fatal(http.ListenAndServe(":80", http.HandlerFunc(redirectTLS)));
		} ()
		log.Fatal(http.ListenAndServeTLS(":443", *cert, *certkey, nil))
	} else {
		log.Println("SSL Configuration not found.")
		log.Fatal(http.ListenAndServe(":80", nil))
	}
}
