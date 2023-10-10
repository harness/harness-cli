package telemetry

import (
	"harness/client"
	"harness/defaults"
	"harness/utils"
)

func Track(trackPayload TrackEventInfoPayload, properties map[string]interface{}) {
	finalPayload := TrackEventPayload{
		TrackPayload: trackPayload,
		Properties:   properties,
		EventType:    EVENT_TYPE_TRACK,
	}
	print("calling track with")
	utils.PrintJson(finalPayload)
	_, _ = client.Post(HARNESS_DIAGNOSTIC_ENDPOINT, "", finalPayload, defaults.CONTENT_TYPE_JSON, nil)
}
