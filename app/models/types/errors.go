package types

import "errors"

var ErrInvalidStatus = errors.New("invalid status")

var ErrStoryDeleted = errors.New("story deleted")

var ErrInvalidStory = errors.New("invalid story")

var ErrInvalidStoryStatusTransition = errors.New("invalid story status transition")
