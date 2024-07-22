package types

import "errors"

var ErrInvalidStatus = errors.New("invalid status")

var ErrStoryDeleted = errors.New("story deleted")

var ErrInvalidStory = errors.New("invalid story")

var ErrInvalidStoryStatusTransition = errors.New("invalid story status transition")

var ErrAnotherStoryAlreadyInProgress = errors.New("another story already in progress")

var ErrAnotherStoryAlreadyInProgress = errors.New("another story already in progress")

var ErrJsonParsingRetriesExceeded = errors.New("json parsing retries exceeded")