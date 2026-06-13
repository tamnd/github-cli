package cli

import (
	"errors"

	"github.com/tamnd/github-cli/github"
)

func isNotFound(err error) bool {
	return errors.Is(err, github.ErrNotFound)
}

func isRateLimit(err error) bool {
	return errors.Is(err, github.ErrRateLimit)
}
