package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/bvarner/pi-launch-control"
	"github.com/dhowden/raspicam"
	"html"
	"log"
	"net/http"
	"os"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/host"
	"strconv"
	"time"
)

var igniter *pi_launch_control.Igniter

var scale *pi_launch_control.Scale

/* Video Camera Settings  */
var videoProfile = *raspicam.NewVid()
var cameraProfile = *raspicam.NewStill()


func CameraControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		errCh := make(chan error)
		go func() {
			for x := range errCh {
				fmt.Fprintf(os.Stderr, "%v\n", x)
			}
		}()
		w.Header().Add("Content-Type", "image/jpeg")
		w.Header().Add("Content-Disposition", "inline; filename=\"rocketstand_" + time.Now().Format(time.RFC3339Nano) + ".jpg\"")
		raspicam.Capture(&cameraProfile, w, errCh)
	}
}

func VideoControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		errCh := make(chan error)
		go func() {
			for x := range errCh {
				fmt.Fprintf(os.Stderr, "%v\n", x)
			}
		}()
		w.Header().Add("Content-Type", "video/h264")
		w.Header().Add("Content-Disposition", "inline; filename=\"rocketstand_" + time.Now().Format(time.RFC3339Nano) + ".h264\"")
		raspicam.Capture(&videoProfile, w, errCh)
	}
}

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

func LaunchControl(w http.ResponseWriter, r *http.Request) {
	force := false
	keys, ok := r.URL.Query()["force"]
	if ok {
		force = len(keys) > 0
	}

	if igniter.IsReady() || force {
		// Push the Camera and data feed file.
		p, ok := w.(http.Pusher)
		if ok {
			p.Push("/camera/video", nil)
			if scale.Initialized && scale.Calibrated {
				p.Push("/scale/capture", nil)
			}
			if force {
				p.Push(fmt.Sprintf("/igniter/countdown?force=%t", force), nil)
			} else {
				p.Push("/igniter/countdown", nil)
			}
		}

		// TODO: Return the document that forces a browser to get the resources...



	} else if (!force) {
		w.Write([]byte("Igniter not ready."));
	}
	return
}

func IgniterCountdownControl(w http.ResponseWriter, r *http.Request) {
	force := false
	keys, ok := r.URL.Query()["force"]
	if ok {
		force = len(keys) > 0
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if ok {
		i := 5
		for i > 0 {
			w.Write([]byte(fmt.Sprintf("%x", i)))
			flusher.Flush();
			time.Sleep(1 * time.Second)
			i--
		}
		w.Write([]byte("Fire"))
		flusher.Flush()
	} else {
		// Wait 5 Seconds, Fire.
		time.Sleep(5 * time.Second)
	}

	w.Write([]byte("Fire"))
	err := igniter.Fire(force)
	if err != nil {
		w.Write([]byte(err.Error()));
		return
	}
}


func ScaleSettingsControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var nscale *pi_launch_control.Scale
		json.NewDecoder(r.Body).Decode(nscale);

		// Alllows us to force a retry of New with the previous device settings if no JSON body is supplied.
		if nscale == nil {
			nscale = scale
		}

		nscale, err := pi_launch_control.NewScale(nscale.Device, nscale.Trigger);
		if err != nil {
			fmt.Println("Error updating scale.", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("500 - Internal Server Error"))
			return
		}
		scale = nscale
	}

	if r.Method == "GET" || r.Method == "POST" {
		json.NewEncoder(w).Encode(scale)
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


func CaptureScaleControl(w http.ResponseWriter, r *http.Request) {
	if scale.Initialized && scale.Calibrated && r.Method == "GET" {
		dur := int(videoProfile.Timeout.Seconds())

		cap, err := scale.Sample(time.Second * time.Duration(dur))
		if err == nil {
			json.NewEncoder(w).Encode(cap)
			return
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}
	} else if scale.Initialized && scale.Calibrated {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"));
	} else {
		w.WriteHeader(http.StatusPreconditionFailed)
		w.Write([]byte("Scale not initialized or calibrated."))
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

	// Initialize the Igniter.
	igniter = &pi_launch_control.Igniter {
		TestPin: gpioreg.ByName("GPIO17"),
		FirePin: gpioreg.ByName("GPIO27"),
	}

	// Initialize the Scale.
	scaleDevice := "/sys/devices/platform/0.weight"
	scaleTrigger := "/sys/bus/iio/devices/iio_sysfs_trigger/trigger0"
	scale, err = pi_launch_control.NewScale(scaleDevice, scaleTrigger);
	if err != nil {
		fmt.Println(err)
	}

	// Initialize the Camera.
	cameraProfile.Width = 640
	cameraProfile.Height = 480
	cameraProfile.Timeout = 300 * time.Millisecond;

	videoProfile.Timeout = 30 * time.Second;
	videoProfile.Width = 640
	videoProfile.Height = 480
	videoProfile.Framerate = 80
	videoProfile.Args = append(videoProfile.Args, "-ae", "10,0xff,0x808000", "-a", "1548", "-a", "\"%Y-%m-%d %X\"", "-pf", "high", "-ih", "-pts")

	fmt.Println("Setting up HTTP server...")
	// Setup the handlers.
	http.HandleFunc("/", func(w http.ResponseWriter, r * http.Request) {
		fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
	})

	http.HandleFunc("/igniter", IgniterControl)
	http.HandleFunc("/igniter/countdown", IgniterCountdownControl)
	http.HandleFunc("/camera", CameraControl)
	http.HandleFunc("/camera/video", VideoControl)
	http.HandleFunc("/scale", ScaleSettingsControl)
	http.HandleFunc("/scale/tare", TareScaleControl)
	http.HandleFunc("/scale/calibrate", CalibrateScaleControl)
	http.HandleFunc("/scale/capture", CaptureScaleControl)

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
