package repository

import (
	"strings"

	"github.com/jaytaylor/gorm"
)

var gormNotFoundErrorString = gorm.ErrRecordNotFound.Error()

func IsRecordNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	result := strings.HasSuffix(err.Error(), gormNotFoundErrorString)
	return result
}
