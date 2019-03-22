package pi_launch_control

import (
	"fmt"
	"github.com/blackjack/webcam"
	"net/http"
	"time"
)

type Camera struct {
	DeviceName	string
	device 		*webcam.Webcam
	pixelFormat webcam.PixelFormat
	trigger		<-chan time.Time


	// Map of clients. Keys = channels over which we can push direct to attached client.
	// Values are actually just meaningless booleans.
	clients map[chan []byte]bool

	// Channel into which new clients can be pushed.
	newClients chan chan []byte

	// Channel into which disconnected clients should be pushed.
	defunctClients chan chan []byte

	// Channel into which message are pushed to be broadcast out to attached clients.
	broadcast   chan []byte

	Initialized bool
}

const FORMAT_MJPG = webcam.PixelFormat((uint32(byte('M'))) | (uint32(byte('J')) << 8) | (uint32(byte('P')) << 16) | (uint32(byte('G')) << 24))

func NewCamera(dev string, trigger <- chan time.Time) (*Camera, error) {
	var err error = nil

	c := new(Camera)
	c.trigger = trigger
	c.DeviceName = dev
	c.device, err = webcam.Open(dev)
	if err != nil {
		return nil, err
	}
	c.clients = make(map[chan []byte]bool)
	c.newClients = make(chan (chan []byte))
	c.defunctClients = make(chan (chan []byte))
	c.broadcast = make(chan []byte)

	// Detect capabilities
	for f := range c.device.GetSupportedFormats() {
		if f == FORMAT_MJPG {
			c.pixelFormat = f
		}
	}

	// Setup capture format
	_, _, _, err = c.device.SetImageFormat(c.pixelFormat, 640, 480)
	if err != nil {
		return nil, err
	}

	// Stuff the device into Streaming mode.
	c.device.SetBufferCount(1)
	err = c.device.StartStreaming()
	if err != nil {
		return nil, err
	}

	// Setup the trigger.
	go c.frameTrigger()

	go c.clientBroadcast()

	c.Initialized = true;

	return c, err
}

func (c *Camera) Close() {
	c.Initialized = false
	// TODO: Wait for condition from clientBroadcast
	fmt.Println("Closing camera.")
	c.device.Close()
}

func (c *Camera) frameTrigger() {
	i := 0
	for range c.trigger {
		buf, idx, err := c.device.GetFrame()
		if err == nil {
			// In single buffer mode we need to copy it.
			// Otherwise, you have to make enough buffers than you can send and re-queue fast enough
			// to not corrupt the mmaped data in the frames.
			// With 256 buffers, there are artifacts in the 640x480 feed at 80hz.
			frame := make([]byte, len(buf))
			copy(frame, buf)
			c.device.ReleaseFrame(idx)

			if i == 0 {
				// Only stream when we hit a 0
				c.broadcast <- frame
			}

			// TODO If Recording...
			// Save the frame to the in-memory hash with the proper filename
			// TODO push the frame, with the proper filename.
			// TODO Emit an event to the emitter signaling that the browser should fetch the still.
			// Browser fetches pushed resource, world is happy.

			i++
			if i == 4 { // Divisor. 80hz / 4 = 20fps for livecast.
				i = 0
			}
		}
	}
}

func (c *Camera) clientBroadcast() {
	for {
		select {
		case s := <- c.newClients:
			c.clients[s] = true
		case s := <- c.defunctClients:
			delete(c.clients, s)
			close(s)
		case frame := <- c.broadcast:
			for s := range c.clients {
				s <- frame
			}
		}
		if !c.Initialized {
			break;
		}
	}
}

func (c *Camera) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		f, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming Unsupported: ", http.StatusInternalServerError)
			return
		}

		// Create a channel the clientBroadcast will send us frames over.
		frameChan := make(chan []byte)

		// Send this to the new clients.
		c.newClients <- frameChan

		notify := r.Context().Done()
		go func() {
			<- notify
			c.defunctClients <- frameChan
		}()

		// Set the headers for the MJPG stream
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Cache-Control","private")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Content-type", "multipart/x-mixed-replace; boundary=frame")
		f.Flush()

		// Loop forever (until the connection is defunct) and send frames.
		for {
			frame, open := <- frameChan
			if !open {
				break;
			}

			// Write the part header
			w.Write([]byte("Content-type: image/jpeg\n\n"))
			// Write the image data
			w.Write(frame)
			// Write the boundary
			w.Write([]byte("--frame\n"))
			f.Flush()
		}
	} else {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("500 - Method Not Supported"))
	}
}