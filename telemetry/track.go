package telemetry

import (
	"harness/client"
	"harness/defaults"
	"harness/shared"
)

func Track(trackPayload TrackEventInfoPayload, properties map[string]interface{}) {
	if trackPayload.UserId == "" {
		trackPayload.UserId = shared.CliCdRequestData.Account
	}
	finalPayload := TrackEventPayload{
		TrackPayload: trackPayload,
		Properties:   properties,
		EventType:    EVENT_TYPE_TRACK,
	}

	_, _ = client.Post(HARNESS_DIAGNOSTIC_ENDPOINT, "", finalPayload, defaults.CONTENT_TYPE_JSON, nil)
}
