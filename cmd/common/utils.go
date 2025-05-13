package common

import (
	"github.com/inhies/go-bytesize"
)

func GetSize(sizeVal int64) string {
	size := bytesize.New(float64(sizeVal))
	return size.String()
}
