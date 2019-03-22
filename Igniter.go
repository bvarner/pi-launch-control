package pi_launch_control

import (
	"errors"
	"periph.io/x/periph/conn/gpio"
	"time"
)

type IgniterState struct {
	Ready	bool
	Firing	bool
	Timestamp	int64
}

/* How we communicate with the Igniter */
type Igniter struct {
	TestPin 	gpio.PinIO	`json:"-"'`
	FirePin		gpio.PinIO
	firing		bool

	Emitter 				`json:"-"`
}

func (i *Igniter) eventName() string {
	return "Igniter"
}

func (i *Igniter) GetState() *IgniterState {
	return &IgniterState{
		i.IsReady(),
		i.IsFiring() || i.firing,
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
