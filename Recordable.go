package pi_launch_control

type Recordable interface {
	StartRecording()
	StopRecording()

	GetRecordedData() map[string][]byte
}
