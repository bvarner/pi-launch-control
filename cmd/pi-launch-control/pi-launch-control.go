package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
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
	Measured	map[int]int
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
			s.devDevice = "/dev/" + f.Name();
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
	s.Measured = make(map[int]int)

	err = s.Tare()
	s.Initialized = err == nil

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





func (s *Scale) Tare() (error) {
	var err error = nil
	s.Lock()

	// Unset calibration data.
	s.Calibrated = false;
	// Always set the first known weight to the scale's tare
	s.ZeroOffset, err = s.Sample(1 * time.Second)
	s.Measured = make(map[int]int)
	s.Measured[0] = s.ZeroOffset

	s.Unlock()

	return err
}

func (s *Scale) Calibrate(mass int) (error) {
	var err error = nil
	s.Lock()

	val, err := s.Sample(3 * time.Second);

	if err == nil {
		s.Measured[mass] = val
	}

	s.Calibrated = len(s.Measured) > 1
	s.Unlock()

	return err
}


func (s *Scale) Sample(duration time.Duration) (int, error) {
	var err error = nil;
	ret := 0

	var wg sync.WaitGroup
	var stop = false;

	DeviceEcho(s.iIODevice + "/buffer/enable", []byte("1"), 0)
	defer DeviceEcho(s.iIODevice + "/buffer/enable", []byte("0"), 0)

	dev, err := os.Open(s.devDevice)
	if err != nil {
		fmt.Println("Unable to open device to read.", err)
		return ret, err
	}
	defer dev.Close()

	data := make([]byte, 16 * 80) // 128 bits @ 80 samples / second.
	data = data[:0]

	wg.Add(2)

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

		// Go.
		for stop != true {
			triggerf.Write(t)
			time.Sleep(12300 * time.Microsecond / 2) // 12500 = 80hz
		}
		// Force the device closed, unblocking any pending Read().
		dev.Close()
	}()

	// Read Thread
	go func() {
		// Notify when done
		defer wg.Done()

		samp := make([]byte, 16) // Single sample

		for stop != true {
			// These block. Hence, the forced dev.Close() Above.
			n, _ := dev.Read(samp)
			if n > 0 {
				data = append(data, samp[0:n]...)
			}
		}
	}()

	// Go.
	time.Sleep(duration)
	stop = true;

	wg.Wait()

	// Data now contains a whole slew of samples.
	// Reset the slice.
	data = data[0:]

	// sample data is 16 bytes (128 bits) per sample.
	// voltage0 makes up 32 bits, 24bits are physically important.
	// voltate1 makes up the next 32 bits. again, 24 bits are physically important. (we ignore these bytes)
	// timestamp makes up the next 64 bits, which should be the unix time the sample was taken, in nanos?
	var off int

	nsamples := len(data) / 16

	if nsamples > 0 {
		// Setup slices for storing values.
		volts0 := make([]uint32, nsamples)
		volts0 = volts0[:0]
		var volt0sum uint32 = 0

		volts1 := make([]uint32, nsamples)
		volts1 = volts1[:0]
		var volt1sum uint32 = 0

		timestamps := make([]int64, nsamples)
		timestamps = timestamps[:0]

		for i := 0; i < nsamples; i++ {
			off = i * 16

			volts0 = append(volts0, binary.LittleEndian.Uint32(data[off+0:off+0+4]))
			volt0sum += volts0[i]
			volts1 = append(volts1, binary.LittleEndian.Uint32(data[off+4:off+4+4]))
			volt1sum += volts1[i]
			timestamps = append(timestamps, tsConvert(data[off+8:off+8+8]))
			//		fmt.Println(fmt.Sprintf("%s @ Volts0: %d, Volts1: %d", time.Unix(0, timestamps[i]), volts0[i], volts1[i]))
		}
		ret = int(volt0sum) / nsamples;
	} else {
		ret = 0
		err = errors.New("Unable to communicate with Scale")
	}

	return ret, err
}

// Returns a int64 from an 8 byte buffer
func tsConvert(b []byte) int64 {
	_ = b[7] // bounds check hint to compiler; see golang.org/issue/14808
	return int64(b[0]) | int64(b[1])<<8 | int64(b[2])<<16 | int64(b[3])<<24 |
		int64(b[4])<<32 | int64(b[5])<<40 | int64(b[6])<<48 | int64(b[7])<<56
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

		// Alllows us to force a retry of New with the previous device settings if no JSON body is supplied.
		if nscale == nil {
			nscale = scale
		}

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
	}

	// Initialize the Camera.

	fmt.Println("Setting up HTTP server...")
	// Setup the handlers.
	http.HandleFunc("/", func(w http.ResponseWriter, r * http.Request) {
		fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
	})

	http.HandleFunc("/igniter", IgniterControl)
	http.HandleFunc("/camera", CameraControl)
	http.HandleFunc("/scale", ScaleSettingsControl)
	http.HandleFunc("/scale/tare", TareScaleControl)
	http.HandleFunc("/scale/calibrate", CalibrateScaleControl)

	http.HandleFunc("/testfire", TestControl)
	http.HandleFunc("/launch", LaunchControl)

	// TODO: Register "/" to serve a web-app.

	log.Fatal(http.ListenAndServe(":80", nil))
}
