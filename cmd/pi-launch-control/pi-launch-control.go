package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/dhowden/raspicam"
	"html"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/host"
	"strconv"
	"strings"
	"sync"
	"time"

	"periph.io/x/periph/conn/gpio"
)

type IgniterState struct {
	Ready	bool
	Firing	bool
	When	time.Time

	igniter	Igniter
}

/* How we communicate with the Igniter */
type Igniter struct {
	TestPin 	gpio.PinIO
	FirePin		gpio.PinIO
}
var igniter *Igniter




type KnownWeight struct {
	Actual		int
	Measured    int
}

/* Scale Settings */
type Scale struct {
	sync.Mutex

	Device		string
	Trigger		string

	iIODevice	string
	devDevice	string
	idx_time	int
	idx_voltage	int

	Initialized	bool

	Calibrated 	bool
	ZeroOffset  int
	Measured	[]KnownWeight
}
var scale *Scale

func NewScale(dev string, triggerDev string) (*Scale, error) {
	var err error = nil

	s := new(Scale)
	s.Device = dev
	s.Trigger = triggerDev

	// Test to make sure the scale device exist.
	if _, err := os.Stat(dev); err != nil {
		return nil, err
	}

	files, err := ioutil.ReadDir(dev)
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		if strings.HasPrefix(f.Name(), "iio:device") {
			s.iIODevice = dev + "/" + f.Name();
			s.Device = "/dev/" + f.Name();
			break;
		}
	}


	// If the sysfs trigger doesn't exist, then we try to create one.
	if _, err := os.Stat(triggerDev); err != nil {

		// Make sure we have the proper sysfs bits.
		if _, err := os.Stat("/sys/bus/iio/devices/iio_sysfs_trigger"); err != nil {
			fmt.Println("Sysfs Triggering Unavilable.", err)
			return s, err
		}

		// Create trigger0 if it doesn't exist.
		if _, err := os.Stat("/sys/bus/iio/devices/iio_sysfs_trigger/trigger0"); err != nil {
			// Create trigger0 since it does not exist
			if err := DeviceEcho("/sys/bus/iio/devices/iio_sysfs_trigger/add_trigger", []byte("0"), 0200); err != nil {
				return s, err
			}
		}
	}

	// By the time we get here we know we have sysfstrigger0
	triggerName, err := ioutil.ReadFile(triggerDev + "/name")

	// Set the trigger as the iio:device trigger.
	if err := DeviceEcho(s.iIODevice + "/trigger/current_trigger", triggerName, 0644); err != nil {
		return s, err
	}

	// Get the timestamp and the voltage0
	DeviceEcho(s.iIODevice + "/scan_elements/in_timestamp_en", []byte("1"), 0644)
	DeviceEcho(s.iIODevice + "/scan_elements/in_voltage0_en", []byte("1"), 0644)

	// Enable the buffer.
	DeviceEcho(s.iIODevice + "/buffer/enable", []byte("1"), 0644)

	// Find out what index the items are.
	buf, err := ioutil.ReadFile(s.iIODevice + "/scan_elements/in_timestamp_index")
	if err != nil {
		return s, err
	}
	s.idx_time, err = strconv.Atoi(string(buf))

	buf, err = ioutil.ReadFile(s.iIODevice + "/scan_elements/in_voltage0_index")
	if err != nil {
		return s, err
	}
	s.idx_voltage, err = strconv.Atoi(string(buf))

	s.ZeroOffset = -1
	s.Measured = make([]KnownWeight, 0, 5)

	err = s.Tare()
	s.Initialized = err == nil

	fmt.Println("Scale Initialized.")

	return s, err
}



func DeviceEcho(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY, perm)
	defer f.Close()
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}
	return err
}





func (s Scale) Tare() error {
	var err error = nil
	s.Lock()

	// Unset calibration data.
	s.Calibrated = false;
	// Always set the first known weight to the scale's tare
	s.ZeroOffset, err = s.Sample(1 * time.Second)
	s.Measured[0] =  KnownWeight {
			Actual: 0,
			Measured: s.ZeroOffset,
	}

	s.Unlock()

	return err
}


func (s Scale) Sample(duration time.Duration) (int, error) {
	var err error = nil;
	ret := 0

	var trigger = sync.NewCond(&s)
	var wg sync.WaitGroup
	var start = false;
	var stop = false;

	wg.Add(3)


	// Trigger thread
	go func() {
		// Notify when done.
		defer wg.Done()

		// Setup sync writing
		triggerf, err := os.OpenFile(s.Trigger + "/trigger_now", os.O_WRONLY | os.O_SYNC, 0)
		if err != nil {
			fmt.Println("Unable to open trigger_now", err)
			return
		}
		defer triggerf.Close()
		t := bytes.NewBufferString("1").Bytes()

		// Wait for it...
		fmt.Println("trigger thread waiting...")
		trigger.L.Lock()
		for (start == false) {
			trigger.Wait()
		}
		trigger.L.Unlock()

		fmt.Println("trigger thread go")

		// Go.
		for stop != true {
			fmt.Println("trigger")
			triggerf.Write(t);
			time.Sleep(12500 * time.Microsecond) // 12500 = 80hz
		}
	}()

	// Read Thread
	go func() {
		// Notify when done
		defer wg.Done()

		dev, err := os.Open(s.devDevice)
		if err == nil {
			fmt.Println("Unable to open device to read.", err)
			return
		}
		defer dev.Close()
		samp := make([]byte, 128) // Single sample
		data := make([]byte, cap(samp) * 80 * int(duration.Seconds())) // 128 bytes @ 80 samples / second.

		// Wait for it...
		fmt.Println("read thread waiting...")
		trigger.L.Lock()
		for start == false {
			trigger.Wait()
		}
		trigger.L.Unlock()

		fmt.Println("read thread go")

		i := 0
		// TODO: Read bytes into the buffer.
		for stop != true {
			i++
			fmt.Println(fmt.Sprintf("%d", i))
			n, err := dev.Read(samp)

			fmt.Println(fmt.Sprintf("read %i bytes", n))
			fmt.Println(hex.EncodeToString(samp[:n]))

			if err != nil {
				if err == io.EOF {
					break;
				}
				fmt.Println(err)
				return
			}
			// move the slice
			data = data[i * cap(samp):]
			// Copy the data
			copy(data, samp)
		}

		// Get the full view.
		data = data[:]

		// TODO: Send the data back in a channel?
		fmt.Println(hex.EncodeToString(data))
	}()

	// Timer thread
	go func() {
		// Notify when done.
		defer wg.Done()


		fmt.Println("timer thread waiting...")
		// Wait for it...
		trigger.L.Lock()
		for start == false {
			trigger.Wait()
		}
		trigger.L.Unlock()
		fmt.Println("timer thread go")

		// Go.
		time.Sleep(duration)

		fmt.Println("Done sleeping.")
		stop = true;
	}()

	// Kick em' off.
	time.Sleep(500 * time.Millisecond)

	fmt.Println("Starting Sample Threads...")
	start = true
	trigger.Broadcast()

	fmt.Println("Waiting for threads to finish.")
	// Wait for it...
	wg.Wait()

	// TODO: Parse the buffer, and compute a reasonable value

	return ret, err
}


/* Video Camera Settings  */
var videoProfile *raspicam.Vid
var cameraProfile *raspicam.Still









func CameraControl(w http.ResponseWriter, r *http.Request) {
	//TODO: Setup the raspicam.vid
	raspicam.NewVid()
}

func TestControl(w http.ResponseWriter, r *http.Request) {
	// TODO: Ensure scale calibration.

	// Spawn thread for triggering scale data collection.

	// Same process as launch control.

	// Stop data collection.

	// Return a location to the h264 stream and data log file.
}

func LaunchControl(w http.ResponseWriter, r *http.Request) {
	// Verify Igniter State.
	// Start Video Recording
	// Initiate countdown.
	// Fire Igniter.

	// Wait for launch success / stop signal.

	// Stop Video Recording.
}




func ScaleSettingsControl(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var nscale *Scale
		json.NewDecoder(r.Body).Decode(nscale);

		nscale, err := NewScale(nscale.Device, nscale.Trigger);
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
	if r.Method == "GET" || r.Method == "POST" {
		scale.Tare()
		json.NewEncoder(w).Encode(scale)
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"));
	}
}


func IgniterControl(w http.ResponseWriter, r *http.Request) {
	var pulse = 0 * time.Nanosecond;

	if r.Method == "POST" {
		for igniter.TestPin.Read() == gpio.Low && pulse < 1 * time.Second {
			pulse += 250 * time.Millisecond;

			igniter.FirePin.Out(gpio.Low)

			igniter.FirePin.Out(gpio.High)
			time.Sleep(pulse)
			igniter.FirePin.Out(gpio.Low)
		}

		if pulse.Nanoseconds() == 0 {
			w.WriteHeader(http.StatusConflict)
		} else if pulse.Seconds() >= 1 {
			w.WriteHeader(http.StatusExpectationFailed)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}

	istate := IgniterState {
		igniter.TestPin.Read() == gpio.Low,
		igniter.FirePin.Read() == gpio.High,
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
	var err error = nil
	if _, err = host.Init(); err != nil {
		log.Fatal(err)
	}

	// Initialize the Igniter.
	igniter = &Igniter {
		TestPin: gpioreg.ByName("GPIO17"),
		FirePin: gpioreg.ByName("GPIO27"),
	}

	// Initialize the Scale.
	scaleDevice := "/sys/devices/platform/0.weight"
	scaleTrigger := "/sys/bus/iio/devices/iio_sysfs_trigger/trigger0"
	scale, err = NewScale(scaleDevice, scaleTrigger);
	if err != nil {
		fmt.Println(err)
		return
	}


	// Initialize the Camera.

	fmt.Println("Setting up HTTP server...")
	// Setup the handlers.
	http.HandleFunc("/", func(w http.ResponseWriter, r * http.Request) {
		fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
	})

	http.HandleFunc("/igniter/", IgniterControl)
	http.HandleFunc("/camera/", CameraControl)
	http.HandleFunc("/scale", ScaleSettingsControl)
	http.HandleFunc("/scale/tare", TareScaleControl)
//	http.HandleFunc("/scale/calibrate/", CalibrateScaleControl)

	http.HandleFunc("/testfire/", TestControl)
	http.HandleFunc("/launch/", LaunchControl)

	// TODO: Register "/" to serve a web-app.

	log.Fatal(http.ListenAndServe(":80", nil))
}
