package model

import (
	"fmt"
	"strings"
)

type AspectRatio string

const (
	AspectRatioLandscape16x9 AspectRatio = "16:9"
	AspectRatioPortrait9x16  AspectRatio = "9:16"
	DefaultAspectRatio       AspectRatio = AspectRatioPortrait9x16
)

func ParseAspectRatio(value string) AspectRatio {
	return AspectRatio(strings.TrimSpace(value))
}

func (a AspectRatio) IsValid() bool {
	switch ParseAspectRatio(string(a)) {
	case AspectRatioLandscape16x9, AspectRatioPortrait9x16:
		return true
	default:
		return false
	}
}

func (a AspectRatio) Normalized() AspectRatio {
	if a.IsValid() {
		return ParseAspectRatio(string(a))
	}

	return DefaultAspectRatio
}

func (a AspectRatio) Dimensions(maxEdge int) (int, int) {
	if maxEdge <= 0 {
		maxEdge = 1280
	}
	shortEdge := maxEdge * 9 / 16
	if shortEdge <= 0 {
		shortEdge = 720
	}

	if a.Normalized() == AspectRatioLandscape16x9 {
		return maxEdge, shortEdge
	}

	return shortEdge, maxEdge
}

func (a AspectRatio) SizeString(maxEdge int) string {
	width, height := a.Dimensions(maxEdge)
	return fmt.Sprintf("%d*%d", width, height)
}
