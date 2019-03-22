package pi_launch_control

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/zfjagann/golang-ring"
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
	TriggerC		<- chan time.Time `json:"-"'`
	readTic			time.Ticker `json:"-"`
	Emitter			`json:"-"`
	sync.Mutex		`json:"-"`
	samples			ring.Ring `json:"-"`
	previousRead	int64

	Device			string
	Trigger			string

	iIODevice  		string
	devDevice  		string
	idxTime    		int
	idxVoltage 		int

	Initialized		bool
	Calibrated 		bool
	Recording 		bool
	// Zero Offset (tare) threshold
	ZeroOffset 		int
	// Known measured values.
	Measured   		map[int]int
	// The adjustment scale value.
	Adjust     		float64

	recordedSamples map[string]Sample
}

type Sample struct {
	Initialized bool
	Calibrated  bool
	Recording   bool
	ZeroOffset	int
	Adjust		float64
	Timestamp	int64
	Volt0		uint32
	Volt0Mass	*float64
	Volt1		uint32
	Volt1Mass	*float64
}


func (s *Sample) CalculateMass() {
	if s.Calibrated {
		v0m := float64(int(s.Volt0)-s.ZeroOffset) / s.Adjust
		v1m := float64(int(s.Volt1)-s.ZeroOffset) / s.Adjust

		s.Volt0Mass = &v0m
		s.Volt1Mass = &v1m
	} else {
		s.Volt0Mass = nil;
		s.Volt1Mass = nil;
	}
}

func NewScale(dev string, trig <- chan time.Time, triggerDev string) (*Scale, error) {
	var err error = nil

	s := new(Scale)
	s.TriggerC = trig
	s.previousRead = 0
	s.EmitterID = s
	s.Device = dev
	s.Trigger = triggerDev
	s.Recording = false

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

	// Disable the buffer and Set the trigger as the iio:device trigger.
	err = deviceEcho(s.iIODevice + "/buffer/enable", []byte("0"), 0)
	if err != nil {
		return s, err
	}
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
	s.idxTime, err = strconv.Atoi(string(buf))

	buf, err = ioutil.ReadFile(s.iIODevice + "/scan_elements/in_voltage0_index")
	if err != nil {
		return s, err
	}
	s.idxVoltage, err = strconv.Atoi(string(buf))

	s.ZeroOffset = -1
	s.Measured = make(map[int]int)
	s.Adjust = 0

	// Go ahead and start reading....
	err = deviceEcho(s.iIODevice + "/buffer/enable", []byte("1"), 0)
	if err != nil {
		return s, err
	}

	// Attempt to open the device.
	s.samples.SetCapacity(80 * 60) // 80 samples / second & average test length

	fd, err := os.Open(s.devDevice)
	if err != nil {
		return s, err
	}
	go s.scaleReadLoop(fd)

	// Begin triggering.
	triggerfd, err := os.OpenFile(s.Trigger + "/trigger_now", os.O_WRONLY | os.O_SYNC, 0)
	if err != nil {
		return s, err
	}

	// Every tick write to the trigger_now file.
	go s.tickerTrigger(triggerfd)

	// Every second emit a value of the current rolling average
	s.readTic = *time.NewTicker(250 * time.Millisecond)
	go s.tickerRead()

	// Tare it up, baby.
	s.Tare()
	s.Initialized = true

	return s, err
}

func (s *Scale) Close() {
	// TODO: Implement some clean-up. :-)
	return
}

func (s *Scale) eventName() string {
	return "Scale"
}

func (s *Scale) tickerTrigger(triggerfd *os.File) {
	for range s.TriggerC {
		triggerfd.Write([]byte("1"))
	}
}

func (s *Scale) StartRecording() {
	s.Recording = true
}

func (s *Scale) StopRecording() {
	s.Recording = false
}

func (s *Scale) tickerRead() {
	for range s.readTic.C {
		s.Read()
	}
}

func (s *Scale) scaleReadLoop(dev *os.File) {
	samp := make([]byte, 16) // Single sample
	for {
		n, _ := dev.Read(samp)
		if n == 16 {
			p := Sample {
				Initialized: s.Initialized,
				Calibrated: s.Calibrated,
				ZeroOffset: s.ZeroOffset,
				Adjust: s.Adjust,

				Timestamp: tsConvert(samp[8:16]),
				Volt0: binary.LittleEndian.Uint32(samp[0:4]),
				Volt1: binary.LittleEndian.Uint32(samp[4:8]),
			}
			p.CalculateMass()
			s.samples.Enqueue(p)

			if s.Recording {
				fmt.Println("Recording scale data")
				// Do this in the background so our Read() loop is _toight_
				go func(scale *Scale, sample Sample) {
					filename := fmt.Sprintf("%d.json", sample.Timestamp)
					scale.recordedSamples[filename] = sample
				}(s, p)
			}
		} else {
			fmt.Println("Read: ", n, " bytes from scale device")
		}
	}
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

func (s *Scale) Tare() {
	s.Lock()
	defer s.Unlock()

	// Reset the ring buffer.
	s.samples.SetCapacity(s.samples.Capacity())
	// Get a rolling average for the Tare reading.
	s.ZeroOffset = int(s.RollingAverage(1 * time.Millisecond).Volt0)

	// Always set the first known weight to the scale's tare
	s.Measured[0] = s.ZeroOffset
}

func (s *Scale) Calibrate(mass int) error {
	s.Lock()
	defer s.Unlock()

	// Make sure we're Tared.
	if len(s.Measured) < 1 {
		return errors.New("scale has not been tared")
	}
	// Reset the ring buffer.
	s.samples.SetCapacity(s.samples.Capacity())
	// Get a rolling average for the mass reading.
	s.Measured[mass] = int(s.RollingAverage(750 * time.Millisecond).Volt0)

	// Compute the adjust values for each mass.
	var accumulated float64 = 0
	discount := 0
	for mass, measured := range s.Measured {
		if mass == 0 {
			discount++
			continue
		}
		accumulated += float64(measured-s.ZeroOffset) / float64(mass)
	}
	s.Adjust = accumulated / float64(len(s.Measured) - discount) // Ignore values that would cause division by zero.

	s.Calibrated = len(s.Measured) > 1

	return nil
}

func (s *Scale) RollingAverage(duration time.Duration) Sample {
	var volt0sum uint32 = 0;
	var volt0mass float64 = 0
	var volt1sum uint32 = 0
	var volt1mass float64 = 0

	start := time.Now().Add(-1 * duration).UnixNano()
	var count uint32 = 0
	var masscount float64 = 0


	for count <= 30 {
		for _, sample := range s.samples.Values() {
			if sample.(Sample).Timestamp >= start {
				volt0sum += sample.(Sample).Volt0
				volt1sum += sample.(Sample).Volt1
				if sample.(Sample).Calibrated {
					masscount ++
					if sample.(Sample).Volt0Mass != nil {
						volt0mass += *sample.(Sample).Volt0Mass
					}
					if sample.(Sample).Volt1Mass != nil {
						volt1mass += *sample.(Sample).Volt1Mass
					}
				}
				count++
			}
		}
	}

	samp := Sample {
		// Scale state
		Initialized: s.Initialized,
		Calibrated: s.Calibrated,
		ZeroOffset: s.ZeroOffset,
		Adjust: s.Adjust,

		// Measured Data
		Timestamp: start,
		Volt0: volt0sum / count,
		Volt0Mass: nil,
		Volt1: volt1sum / count,
		Volt1Mass: nil,
	}
	if volt0mass > 0 {
		v0m := volt0mass / masscount
		samp.Volt0Mass = &v0m
	}
	if volt1mass > 0 {
		v1m := volt1mass / masscount
		samp.Volt1Mass = &v1m
	}

	return samp
}

func (s *Scale) Read() Sample {
	start := s.previousRead
	if start == 0 {
		start = time.Now().UnixNano()
	}
	end := time.Now().UnixNano()
	unreported := make([]Sample, 0) // Start empty so that append does the sensible thing.

	for _, sample := range s.samples.Values() {
		if sample.(Sample).Timestamp >= start {
			unreported = append(unreported, sample.(Sample))
		}
	}
	s.Emit(unreported)

	s.previousRead = end
	return s.RollingAverage(time.Duration(end - start))
}

// Returns a int64 from an 8 byte buffer
func tsConvert(b []byte) int64 {
	_ = b[7] // bounds check hint to compiler; see golang.org/issue/14808
	return int64(b[0]) | int64(b[1])<<8 | int64(b[2])<<16 | int64(b[3])<<24 |
		int64(b[4])<<32 | int64(b[5])<<40 | int64(b[6])<<48 | int64(b[7])<<56
}
