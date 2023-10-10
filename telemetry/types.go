package telemetry

type TrackEventInfoPayload struct {
	UserId    string `json:"userId"`
	EventName string `json:"eventName"`
}

type TrackEventPayload struct {
	EventType    string                `json:"eventType"`
	Properties   interface{}           `json:"properties"`
	TrackPayload TrackEventInfoPayload `json:"trackPayload"`
}

const (
	EVENT_TYPE_TRACK string = "track"
)
