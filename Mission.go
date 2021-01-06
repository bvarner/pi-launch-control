package pi_launch_control

import (
	"encoding/json"
	"fmt"
	"time"
)

type Mission struct {
	broker			*Broker
	sequenceTicker 	*time.Ticker

	Timestamp		int64
	Clock	 		int
	Aborted		   	bool
	Complete 		bool

	igniter         *Igniter
	scale 			*Scale
	camera 			*Camera
}

func NewMission(igniter *Igniter, scale *Scale, camera *Camera) *Mission {
	m := &Mission {
		broker: nil,
		sequenceTicker: nil,

		Timestamp: time.Now().UnixNano(),
		Clock: -10,
		Aborted: false,
		Complete: false,

		igniter: igniter,
		scale: scale,
		camera: camera,
	}
	return m
}

func (m *Mission) mission() {
	m.Clock = -10 // 10 Second Countdown
	for range m.sequenceTicker.C {
		if !m.Aborted {
			// At t - 3, start recording.
			if m.Clock == -3 {
				// Igniter First.
				m.igniter.StartRecording()
				// Scale Second.
				if m.scale.Initialized {
					m.scale.StartRecording()
				}
				// Camera Last.
				if m.camera.Initialized {
					m.camera.StartRecording()
				}
			}

			// anytime before ignition the igniter fails,
			if m.Clock <= 0 && !m.igniter.IsReady() {
				m.Aborted = true
			}

			// At Zero, Fire if not aborted.
			if m.Clock == 0 && !m.Aborted {
				m.igniter.Fire()
			}

			// When the clock is +12, Mission Complete.
			if m.Clock >= 12 {
				m.Complete = true
				m.stop()
			}
		}

		if m.Aborted {
			m.stop()
		}

		b, err := json.Marshal(m)
		if err == nil {
			s := fmt.Sprintf("event: %s\ndata: %s\n", "Mission", string(b))
			m.broker.Outgoing <- s
		}
		m.Clock++

		// Clean up
		if m.sequenceTicker == nil {
			m.broker = nil
			break
		}
	}
}


func (m *Mission) Start(broker *Broker) {
	m.broker = broker
	m.sequenceTicker = time.NewTicker(1 * time.Second)
	go m.mission()
}

func (m *Mission) stop() {
	m.sequenceTicker.Stop()
	m.sequenceTicker = nil

	// Igniter Last. (inverse order)
	if m.camera.Initialized {
		m.camera.StopRecording()
	}
	if m.scale.Initialized {
		m.scale.StopRecording()
	}
	m.igniter.StopRecording()
}

func (m *Mission) Abort() {
	m.Aborted = true;
}
