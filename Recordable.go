package pi_launch_control

import "archive/zip"

type Recordable interface {
	StartRecording()
	StopRecording()

	GetRecordedData() map[*zip.FileHeader][]byte
}
