package pi_launch_control

import (
	"errors"
	"periph.io/x/periph/conn/gpio"
	"time"
)

type IgniterState struct {
	Ready	bool
	Firing	bool
	When	time.Time
}

/* How we communicate with the Igniter */
type Igniter struct {
	TestPin 	gpio.PinIO
	FirePin		gpio.PinIO
}

func (i *Igniter) GetState() *IgniterState {
	return &IgniterState{
		i.IsReady(),
		i.IsFiring(),
		time.Now(),
	}
}

func (i *Igniter) IsReady() (bool) {
	return i.TestPin.Read() == gpio.Low
}

func (i *Igniter) IsFiring() bool {
	return i.FirePin.Read() == gpio.High
}

func (i *Igniter) Fire(force bool) (error) {
	var pulse time.Duration = 0

	// Pulse up to 1 second.
	for (i.IsReady() || force) && pulse.Seconds() < 1 {
		pulse += 250 * time.Millisecond

		i.FirePin.Out(gpio.Low)

		i.FirePin.Out(gpio.High)
		time.Sleep(pulse)
		i.FirePin.Out(gpio.Low)

		time.Sleep(500 * time.Millisecond) // half-second between pulses.
	}

	// Never fired, not forced.
	if pulse == 0 {
		return errors.New("igniter not ready")
	}

	// Did it burn through in the proper amount of time?
	if pulse.Seconds() >= 1 {
		if i.IsReady() {
			return errors.New("igniter failed to burn through")
		} else {
			// TODO: Igniter burnt through.
		}
	}

	return nil
}

