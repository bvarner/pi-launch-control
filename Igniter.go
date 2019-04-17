package pi_launch_control

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"log"
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/host"
	"sync"
	"time"
)

type IgniterState struct {
	Ready		bool
	Firing		bool
	Recording 	bool
	Timestamp	int64
}

/* How we communicate with the Igniter */
type Igniter struct {
	TestPin 	gpio.PinIO	`json:"-"'`
	FirePin		gpio.PinIO	`json:"-"`

	firing		bool
	Recording 	bool

	Emitter 				`json:"-"`
	Recordable				`json:"-"`
	sync.Mutex				`json:"-"`

	recordedState	[]IgniterState
}

func NewIgniter(testPinName string, firePinName string)(*Igniter, error) {
	var err error = nil;
	if _, err = host.Init(); err != nil {
		log.Fatal(err)
	}

	i := &Igniter{
		TestPin: gpioreg.ByName(testPinName),
		FirePin: gpioreg.ByName(firePinName),
	}
	i.EmitterID = i

	// Set it to pull high, so contact sinks to ground. Interrupt on both edges.
	err = i.TestPin.In(gpio.PullUp, gpio.BothEdges)
	if err == nil {
		go func() {
			for {
				// If an edge changes
				i.TestPin.WaitForEdge(-1)
				i.Emit(i.GetState())
			}
		}()
	} else {
		log.Print(err)
	}

	return i, err
}

func (i *Igniter) eventName() string {
	return "Igniter"
}

func (i *Igniter) ResetRecording() {
	i.Lock()
	defer i.Unlock()

	i.Recording = false
	i.recordedState = nil
	i.recordedState = make([]IgniterState, 0)
}

func (i *Igniter) StartRecording() {
	i.Lock()
	defer i.Unlock()

	i.Recording = false
	i.recordedState = nil
	i.recordedState = make([]IgniterState, 0)
	i.Recording = true

	i.Emit(i.GetState())
}


func (i *Igniter) StopRecording() {
	i.Lock()
	defer i.Unlock()
	i.Recording = false

	i.Emit(i.GetState())
}

func (i *Igniter) GetRecordedData() map[*zip.FileHeader][]byte {
	i.Lock()
	defer i.Unlock()

	files := make(map[*zip.FileHeader][]byte)
	header := &zip.FileHeader {
		Name:   "igniter.json",
		Modified: time.Unix(0, i.recordedState[0].Timestamp),
		Method: zip.Deflate,
	}

	files[header], _ = json.Marshal(i.recordedState)
	return files
}

func (i *Igniter) GetFirstRecorded() *IgniterState {
	if i.recordedState != nil {
		return &(i.recordedState[0])
	}
	return nil
}

func (i *Igniter) GetState() IgniterState {
	return IgniterState{
		i.IsReady(),
		i.IsFiring() || i.firing,
		i.Recording,
		time.Now().Unix(),
	}
}

func (i *Igniter) IsReady() (bool) {
	return i.TestPin.Read() == gpio.Low
}

func (i *Igniter) IsFiring() bool {
	return i.FirePin.Read() == gpio.High
}

func (i *Igniter) Fire() (error) {
	i.firing = true;
	var pulse time.Duration = 0

	// Pulse up to 1 second.
	for i.IsReady() && pulse.Seconds() < 1 {
		pulse += 250 * time.Millisecond

		i.FirePin.Out(gpio.Low)
		i.FirePin.Out(gpio.High)
		i.Emit(i.GetState())
		time.Sleep(pulse)

		i.FirePin.Out(gpio.Low)
		i.Emit(i.GetState())
		time.Sleep(500 * time.Millisecond) // half-second between pulses.
	}
	i.firing = false;
	i.Emit(i.GetState())

	// Never fired, not forced.
	if pulse == 0 {
		return errors.New("igniter not ready")
	}

	// Event stream should be emitting the proper current state of the igniter.
	return nil
}

func (i *Igniter) Emit(v interface{}) {
	if i.Recording {
		go func(igniter *Igniter, state IgniterState) {
			igniter.Lock()
			defer igniter.Unlock()

			if state.Recording {
				igniter.recordedState = append(igniter.recordedState, state)
			}
		}(i, v.(IgniterState))
	}
	i.Emitter.Emit(v)
}