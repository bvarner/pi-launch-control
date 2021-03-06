// pi-launch-control API.
//
// Provides a means to control a model rocket motor test stand.
//
// Schemes: https
// Host: localhost
// BasePath: /
// Consumes:
// - application/json
//
// Produces:
// - application/json
//
// swagger:meta
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
	"os/exec"
	"strconv"
	"time"
)

var mission *pi_launch_control.Mission

var igniter *pi_launch_control.Igniter

var scale *pi_launch_control.Scale

var camera *pi_launch_control.Camera

var broker *pi_launch_control.Broker

var handler http.Handler

// swagger:operation GET /scale getScale
//
// Returns the scale state.
//
// ---
// produces:
// - application/json
// responses:
//   '200':
//     description: scale response
//     schema:
//       "$ref": "#/definitions/Scale"
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

// swagger: operation GET /scale/tare
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

func ClockControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		keys, ok := r.URL.Query()["tstamp"]
		if ok {
			timestamp, err := strconv.ParseUint(keys[0], 10, 64)
			args := []string{fmt.Sprintf("@%d", timestamp)}
			_, err = exec.Command("date", args...).Output()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"))
	}
}

func IgniterControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
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
		if mission != nil {
			if mission.Aborted || mission.Complete {
				// Make sure we call this from the server side.
				mission.Abort()
				mission = nil
				// So that we can carry on now with a new mission.
			} else {
				w.WriteHeader(http.StatusExpectationFailed)
				w.Write([]byte("417 - Mission Already Underway"))
				return
			}
		}

		// Check to verify Igniter is OK.
		if !igniter.IsReady() {
			w.WriteHeader(http.StatusExpectationFailed)
			w.Write([]byte("417 - Check Igniter Connections."))
			return
		}

		mission = pi_launch_control.NewMission(igniter, scale, camera)
		mission.Start(broker)
	case "/mission/abort":
		if mission == nil {
			w.WriteHeader(http.StatusExpectationFailed)
			w.Write([]byte("417 - No Mission in Progress"))
			return
		}

		mission.Abort()
		mission = nil
	case "/mission/download":
		if r.Method == "GET" {
			buf := new(bytes.Buffer)
			filename := ""

			if igniter.GetFirstRecorded() != nil {
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
					total += len(devices[len(devices)-1])
				}
				if camera.Initialized {
					devices = append(devices, camera.GetRecordedData())
					total += len(devices[len(devices)-1])
				}

				filename = fmt.Sprintf("%d", igniter.GetFirstRecorded().Timestamp)

				// If we have a name query param, add it.
				namekeys, ok := r.URL.Query()["name"]
				if ok {
					filename = fmt.Sprintf("%s-%s", filename, namekeys[0])
				}

				// Append the .zip.
				filename = fmt.Sprintf("%s.zip", filename)
				// And away we go.
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

	// Create a channel for the scale and the camera triggers
	scaleTrigC := make(chan time.Time, 1)
	camTrigC   := make(chan time.Time, 1)

	// Setup the SSE Broker for event data.
	broker = pi_launch_control.NewBroker()
	broker.Start()

	// Startup sequence here is important.
	// The periph.io sysfs driver will export every gpio in the system to sysfs (ugh!)
	// which ends up allocating all those gpios as exported, in-use.
	// When the scale device tries to open() for the first time, it cannot get the sck and data pins,
	// as they're already tied up with sysfs exports.
	// As such, the only way to 'fix' this is to either set those pins as hogs in the device tree (preferred)
	// or, to initialize the scale first, then the igniter.
	// Of course, I could just write a kernel driver for the igniter...
	
	// Initialize the Scale.
	scaleDevice := "/sys/devices/platform/weight@0"
	scaleTrigger := "/sys/bus/iio/devices/iio_sysfs_trigger/trigger0"
	scale, err = pi_launch_control.NewScale(scaleDevice, scaleTrigC, scaleTrigger);
	if err != nil {
		fmt.Println(err)
		fmt.Println("Scale not Initialized: ", err)
	} else {
		scale.AddListener(broker.Outgoing)
		fmt.Println("Scale Present")
		defer scale.Close()
	}

	// Initialize the Igniter.
	igniter, err = pi_launch_control.NewIgniter("GPIO17", "GPIO27")
	if err != nil {
		fmt.Println(err)
		fmt.Println("Igniter not Initialized: ", err)
	} else {
		igniter.AddListener(broker.Outgoing)
		fmt.Println("Igniter Initialized")
	}

	// Initialize the Camera
	camera, err = pi_launch_control.NewCamera("/dev/video0", camTrigC)
	if err != nil {
		fmt.Println("Camera not Initialized: ", err)
	} else {
		camera.AddListener(broker.Outgoing)
		fmt.Println("Camera Present")
		defer camera.Close()
	}

	// Setup the Ticker for triggering scale and image capture.
	devicePoller := time.NewTicker(12500 * time.Microsecond) // 80hz
	// Go func to send to both of them when devicePoller ticks
	go func() {
		for t := range devicePoller.C {
			if scale != nil && scale.Initialized {
				scaleTrigC <- t
			}
			if camera != nil && camera.Initialized {
				camTrigC <- t
			}
		}
	}()

	// Setup no initial Mission
	mission = nil

	fmt.Println("Setting up HTTP server...")

	handler = http.FileServer(rice.MustFindBox("webroot").HTTPBox())
	fmt.Println("Found the rice box.")

	// Setup the handlers.
	http.HandleFunc("/", RootHandler)

	// Setup the SSE Event Handler. This comes from the 'broker'.
	http.HandleFunc("/events", broker.ServeHTTP)

	http.HandleFunc("/igniter", IgniterControl)

	http.HandleFunc("/camera", camera.ServeHTTP)
	http.HandleFunc("/camera/status", CameraStatusControl)

	http.HandleFunc("/clock", ClockControl)

	http.HandleFunc("/scale", ScaleSettingsControl)
	http.HandleFunc("/scale/tare", TareScaleControl)
	http.HandleFunc("/scale/calibrate", CalibrateScaleControl)

	http.HandleFunc("/mission/", MissionControl)

	cert := flag.String("cert", "/etc/ssl/certs/pi-launch-control/cert.pem", "The certificate for this server.")
	certkey := flag.String("key", "/etc/ssl/certs/pi-launch-control/key.pem", "The key for the server cert.")

	flag.Parse()

	_, certerr := os.Stat(*cert)
	_, keyerr := os.Stat(*certkey)

	if certerr == nil && keyerr == nil {
		fmt.Println("SSL Configuration set up.")
		go func() {
			log.Fatal(http.ListenAndServe(":80", http.HandlerFunc(redirectTLS)));
		} ()
		log.Fatal(http.ListenAndServeTLS(":443", *cert, *certkey, nil))
	} else {
		fmt.Println("SSL Configuration not found.")
		log.Fatal(http.ListenAndServe(":80", nil))
	}
}
