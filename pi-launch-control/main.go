package main

import (
	"archive/zip"
	"bytes"
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

var sequenceTicker *time.Ticker

var igniter *pi_launch_control.Igniter

var scale *pi_launch_control.Scale

var camera *pi_launch_control.Camera

var broker *pi_launch_control.Broker

var handler http.Handler

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

// Launch / Test Sequence Control.
func MissionControl(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/mission/start":
		if sequenceTicker != nil {
			w.WriteHeader(http.StatusExpectationFailed)
			w.Write([]byte("417 - Mission Already Underway"))
			return
		}

		// Igniter First.
		igniter.StartRecording()
		if scale.Initialized {
			scale.StartRecording()
		}
		if camera.Initialized {
			camera.StartRecording()
		}

		// Setup a ticker
		sequenceTicker = time.NewTicker(1 * time.Second)
		go func(brok *pi_launch_control.Broker) {
			i := 10 // 10 Second Countdown
			for t := range sequenceTicker.C {
				if i >= 0 {
					obj := map[string]interface{}{
						"Timestamp": t.UnixNano(),
						"Remaining": i,
						"Aborted":   false,
					}

					b, err := json.Marshal(obj)
					if err == nil {
						s := fmt.Sprintf("event: %s\ndata: %s\n", "Sequence", string(b))
						brok.Outgoing <- s
					}
					i = i - 1
				} else {
					sequenceTicker.Stop()
					break;
				}
			}
		}(broker)
	case "/mission/stop":
		if sequenceTicker == nil {
			w.WriteHeader(http.StatusExpectationFailed)
			w.Write([]byte("417 - No Mission in Progress"))
			return
		}
		sequenceTicker.Stop()
		sequenceTicker = nil

		// Igniter Last. (inverse order)
		if camera.Initialized {
			camera.StopRecording()
		}
		if scale.Initialized {
			scale.StopRecording()
		}
		igniter.StopRecording()
	case "/mission/abort":
		if sequenceTicker == nil {
			w.WriteHeader(http.StatusExpectationFailed)
			w.Write([]byte("417 - No Mission in Progress"))
			return
		}
		// Igniter Last. (inverse order)
		if camera.Initialized {
			camera.StopRecording()
		}
		if scale.Initialized {
			scale.StopRecording()
		}
		igniter.StopRecording()

		sequenceTicker.Stop()
		sequenceTicker = nil

		obj := map[string]interface{}{
			"Timestamp": time.Now().UnixNano(),
			"Aborted":   true,
		}

		b, err := json.Marshal(obj)
		if err == nil {
			s := fmt.Sprintf("event: %s\ndata: %s\n", "Sequence", string(b))
			broker.Outgoing <- s
		}
	case "/mission/download":
		if r.Method == "GET" {
			buf := new(bytes.Buffer)
			zw := zip.NewWriter(buf)

			// Create an array / slice of devices to get data from.
			// If we get an error on doing anything with a device file, we bail.
			var err error = nil

			devices := make([]map[*zip.FileHeader][]byte, 1)

			total := 0
			complete := 0

			// Always add the igniter.
			devices[0] = igniter.GetRecordedData()
			total += len(devices[0])
			if scale.Initialized {
				devices = append(devices, scale.GetRecordedData())
				total += len(devices[len(devices) - 1])
			}
			if camera.Initialized {
				devices = append(devices, camera.GetRecordedData())
				total += len(devices[len(devices) - 1])
			}

			filename := ""
			if igniter.GetFirstRecorded() != nil {
				filename = fmt.Sprintf("%d.zip", igniter.GetFirstRecorded().Timestamp)
				for _, data := range devices {
					if err != nil {
						break;
					}
					//
					for fname, fdata := range data {
						f, ferr := zw.CreateHeader(fname)
						if ferr != nil {
							err = ferr
							break
						}
						_, ferr = f.Write(fdata)
						if ferr != nil {
							err = ferr
							break
						}
						complete++;
						obj := map[string]interface{}{
							"Total":    total,
							"Complete": complete,
							"Error":    nil,
						}

						statusdata, err := json.Marshal(obj)
						if err == nil {
							s := fmt.Sprintf("event: %s\ndata: %s\n", "MissionPacking", string(statusdata))
							broker.Outgoing <- s
						}
					}
				}

				// Still no error?
				if err == nil {
					err = zw.Close()
				}

				if err != nil {
					obj := map[string]interface{}{
						"Total":    total,
						"Complete": complete,
						"Error":    nil,
					}

					statusdata, err := json.Marshal(obj)
					if err == nil {
						s := fmt.Sprintf("event: %s\ndata: %s\n", "MissionPacking", string(statusdata))
						broker.Outgoing <- s
					}

					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(err.Error()))
					return
				}
			}

			if filename == "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			w.Header().Add("Pragma", "public")
			w.Header().Add("Expires", "0")
			w.Header().Add("Cache-Control", "must-revalidate, post-check=0, pre-check=0")
			w.Header().Add("Cache-Control", "public")
			w.Header().Add("Content-type", "application/octet-stream")
			w.Header().Add("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
			w.Header().Add("Content-Transfer-Encoding", "binary")
			w.Header().Add("Content-Length", fmt.Sprintf("%d", buf.Len()))
			w.Write(buf.Bytes())
			return
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("500 - Method Not Supported"))
		}
	}
	w.WriteHeader(http.StatusOK)
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
	devicePoller := time.NewTicker(12500 * time.Microsecond) // 80hz
	// Create a channel for the scale and the camera.
	scaleTrigC := make(chan time.Time, 1)
	camTrigC   := make(chan time.Time, 1)
	// Go func to send to both of them when devicePoller ticks
	go func() {
		for t := range devicePoller.C {
			scaleTrigC <- t
			camTrigC <- t
		}
	}()

	// Setup the SSE Broker for event data.
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

	http.HandleFunc("/mission/", MissionControl)

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
