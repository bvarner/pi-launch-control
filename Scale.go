package pi_launch_control

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

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
type ScaleCapture struct {
	ZeroOffset  int
	Measured	map[int]int

	Capture		[]Sample
}

type Sample struct {
	Timestamp	int64
	Volt0		uint32
	Volt1		uint32
}

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
			if err := deviceEcho("/sys/bus/iio/devices/iio_sysfs_trigger/add_trigger", []byte("0"), 0200); err != nil {
				return s, err
			}
		}
	}

	// By the time we get here we know we have sysfstrigger0
	triggerName, err := ioutil.ReadFile(triggerDev + "/name")

	// Set the trigger as the iio:device trigger.
	if err := deviceEcho(s.iIODevice + "/trigger/current_trigger", triggerName, 0); err != nil {
		return s, err
	}

	// Get the timestamp and the voltage0
	deviceEcho(s.iIODevice + "/scan_elements/in_timestamp_en", []byte("1"), 0644)
	deviceEcho(s.iIODevice + "/scan_elements/in_voltage0_en", []byte("1"), 0644)

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


func deviceEcho(filename string, data []byte, perm os.FileMode) error {
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
	s.ZeroOffset, err = s.SampleAvg(1 * time.Second)
	s.Measured = make(map[int]int)
	s.Measured[0] = s.ZeroOffset

	s.Unlock()

	return err
}

func (s *Scale) Calibrate(mass int) (error) {
	var err error = nil
	s.Lock()

	val, err := s.SampleAvg(3 * time.Second);

	if err == nil {
		s.Measured[mass] = val
	}

	s.Calibrated = len(s.Measured) > 1
	s.Unlock()

	return err
}

func (s *Scale) Sample(duration time.Duration) (*ScaleCapture, error) {
	timestamps, volts0, volts1, err := s.sample(duration)

	if err != nil {
		return nil, err;
	}
	i := 0
	samples := make([]Sample, len(timestamps))
	for i < len(timestamps) {
		samples[i] = Sample {
			timestamps[i],
			volts0 [i],
			volts1[i],
		}
		i++
	}

	return &ScaleCapture {
		ZeroOffset: s.ZeroOffset,
		Measured: s.Measured,
		Capture: samples,
	}, nil
}

func (s *Scale) sample(duration time.Duration) ([]int64, []uint32, []uint32, error) {
	var err error = nil;

	var wg sync.WaitGroup
	var stop = false;

	deviceEcho(s.iIODevice + "/buffer/enable", []byte("1"), 0)
	defer deviceEcho(s.iIODevice + "/buffer/enable", []byte("0"), 0)

	dev, err := os.Open(s.devDevice)
	if err != nil {
		fmt.Println("Unable to open device to read.", err)
		return nil, nil, nil, err
	}
	defer dev.Close()

	data := make([]byte, 16 * 80 * (int(duration.Seconds() + 1))) // 128 bits @ 80 samples / second.
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

		volts1 := make([]uint32, nsamples)
		volts1 = volts1[:0]

		timestamps := make([]int64, nsamples)
		timestamps = timestamps[:0]

		for i := 0; i < nsamples; i++ {
			off = i * 16

			volts0 = append(volts0, binary.LittleEndian.Uint32(data[off+0:off+0+4]))
			volts1 = append(volts1, binary.LittleEndian.Uint32(data[off+4:off+4+4]))
			timestamps = append(timestamps, tsConvert(data[off+8:off+8+8]))
			//		fmt.Println(fmt.Sprintf("%s @ Volts0: %d, Volts1: %d", time.Unix(0, timestamps[i]), volts0[i], volts1[i]))
		}

		return timestamps, volts0, volts1, nil
	} else {
		err = errors.New("unable to communicate with scale")
	}
	return nil, nil, nil, err
}


// Sample and return the average reading over a 3 second period.
func (s *Scale) SampleAvg(duration time.Duration) (int, error) {
	ret := 0;
	timestamps, volts0, volts1, err := s.sample(duration)

	if err == nil {
		nsamples := len(timestamps)

		var volt0sum uint32 = 0
		var volt1sum uint32 = 0

		for i := 0; i < nsamples; i++ {
			volt0sum += volts0[i]
			volt1sum += volts1[i]
		}

		ret = int(volt0sum) / nsamples;
	}

	return ret, err
}

// Returns a int64 from an 8 byte buffer
func tsConvert(b []byte) int64 {
	_ = b[7] // bounds check hint to compiler; see golang.org/issue/14808
	return int64(b[0]) | int64(b[1])<<8 | int64(b[2])<<16 | int64(b[3])<<24 |
		int64(b[4])<<32 | int64(b[5])<<40 | int64(b[6])<<48 | int64(b[7])<<56
}
