package pi_launch_control

import "archive/zip"

type Recordable interface {
	StartRecording()
	StopRecording()
	ResetRecording()

	GetRecordedData() map[*zip.FileHeader][]byte
}
