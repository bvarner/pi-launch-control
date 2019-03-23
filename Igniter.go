package pi_launch_control

import (
	"encoding/json"
	"errors"
	"fmt"
	"periph.io/x/periph/conn/gpio"
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

	recordedState	map[string]IgniterState
}

func (i *Igniter) eventName() string {
	return "Igniter"
}

func (i *Igniter) StartRecording() {
	i.Lock()
	defer i.Unlock()

	i.Recording = false
	i.recordedState = nil
	i.recordedState = make(map[string]IgniterState)
	i.Recording = true

	i.Emit(i.GetState())
}


func (i *Igniter) StopRecording() {
	i.Lock()
	defer i.Unlock()
	i.Recording = false

	i.Emit(i.GetState())
}

func (i *Igniter) GetRecordedData() map[string][]byte {
	i.Lock()
	defer i.Unlock()

	files := make(map[string][]byte)
	for fname, state := range i.recordedState {
		files[fname], _ = json.Marshal(state)
	}

	return files
}

func (i *Igniter) GetState() *IgniterState {
	return &IgniterState{
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

func (i *Igniter) Fire(force bool) (error) {
	i.firing = true;
	var pulse time.Duration = 0

	// Pulse up to 1 second.
	for (i.IsReady() || force) && pulse.Seconds() < 1 {
		pulse += 250 * time.Millisecond

		i.FirePin.Out(gpio.Low)
		i.Emit(i.GetState())

		i.FirePin.Out(gpio.High)
		i.Emit(i.GetState())

		time.Sleep(pulse)
		i.FirePin.Out(gpio.Low)
		i.Emit(i.GetState())

		time.Sleep(500 * time.Millisecond) // half-second between pulses.
	}
	i.firing = false;

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
				filename := fmt.Sprintf("%d-igniter.json", state.Timestamp)
				igniter.recordedState[filename] = state
			}
		}(i, v.(IgniterState))
	}
	i.Emitter.Emit(v)
}