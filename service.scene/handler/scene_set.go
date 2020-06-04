package handler

import (
	"github.com/jakewright/home-automation/libraries/go/database"
	"github.com/jakewright/home-automation/libraries/go/oops"
	scenedef "github.com/jakewright/home-automation/service.scene/def"
	"github.com/jakewright/home-automation/service.scene/domain"
)

// SetScene emits an event to trigger the scene to be set asynchronously
func (c *Controller) SetScene(r *request, body *scenedef.SetSceneRequest) (*scenedef.SetSceneResponse, error) {
	scene := &domain.Scene{}
	if err := database.Find(&scene, body.SceneId); err != nil {
		return nil, err
	}

	if scene == nil {
		return nil, oops.NotFound("Scene not found")
	}

	if err := (&scenedef.SetSceneEvent{
		SceneId: body.SceneId,
	}).Publish(); err != nil {
		return nil, err
	}

	return &scenedef.SetSceneResponse{}, nil
}
