package main

import (
	"image"
	"time"

	"gocv.io/x/gocv"
)

type template struct {
	threshold float32
	mat       gocv.Mat
	mask      gocv.Mat
	keypoints []gocv.KeyPoint
}

type takedowninfo struct {
	timeout         time.Time
	initialposition image.Point
	positions       []image.Point
	zaps            []image.Point
	seencount       int
}

type result struct {
	name       string
	confidence float32
	threshold  float32
	location   image.Point
	rect       image.Rectangle
}
