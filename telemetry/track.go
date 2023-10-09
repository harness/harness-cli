package telemetry

import (
	"harness/client"
	"harness/defaults"
)

func Track(trackPayload TrackEventInfoPayload, properties map[string]interface{}) {
	finalPayload := TrackEventPayload{
		TrackPayload: trackPayload,
		Properties:   properties,
		EventType:    EVENT_TYPE_TRACK,
	}
	_, _ = client.Post(HARNESS_DIAGNOSTIC_ENDPOINT, "", finalPayload, defaults.CONTENT_TYPE_JSON, nil)
}
