// Code generated by protoc-gen-jrpc. DO NOT EDIT.

package sceneproto

import (
	"encoding/json"

	"github.com/jakewright/home-automation/libraries/go/errors"
	"github.com/jakewright/home-automation/libraries/go/firehose"
)

// Publish publishes the event to the Firehose
func (e *SetSceneEvent) Publish() error {
	if err := e.Validate(); err != nil {
		return err
	}

	return firehose.Publish("set-scene", e)
}

type SetSceneEventHandler func(*SetSceneEvent) firehose.Result

func (h SetSceneEventHandler) EventName() string {
	return "set-scene"
}

func (h SetSceneEventHandler) HandleEvent(e *firehose.Event) firehose.Result {
	var body SetSceneEvent
	if err := json.Unmarshal(e.Payload, &body); err != nil {
		return firehose.Discard(errors.WithMessage(err, "failed to unmarshal payload"))
	}
	return h(&body)
}
