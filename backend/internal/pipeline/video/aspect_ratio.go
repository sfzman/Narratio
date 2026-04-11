package video

import (
	"fmt"
	"math"

	"github.com/sfzman/Narratio/backend/internal/model"
)

func resolveTaskAspectRatio(payload map[string]any) model.AspectRatio {
	return resolveAspectRatio(payload, model.AspectRatioLandscape16x9)
}

func resolveAspectRatio(
	payload map[string]any,
	fallback model.AspectRatio,
) model.AspectRatio {
	if payload == nil {
		return fallback
	}

	value, ok := payload["aspect_ratio"]
	if !ok {
		return fallback
	}

	s, ok := value.(string)
	if !ok {
		return fallback
	}

	aspectRatio := model.ParseAspectRatio(s)
	if !aspectRatio.IsValid() {
		return fallback
	}

	return aspectRatio.Normalized()
}

func buildCoverScaleFilter(aspectRatio model.AspectRatio) string {
	width, height := aspectRatio.Dimensions(defaultFinalVideoMaxEdge)
	return fmt.Sprintf(
		"scale=w='ceil(max(%d/iw\\,%d/ih)*iw/2)*2':h='ceil(max(%d/iw\\,%d/ih)*ih/2)*2',crop=%d:%d,setsar=1,fps=%d,format=yuv420p",
		width,
		height,
		width,
		height,
		width,
		height,
		defaultFinalVideoFPS,
	)
}

func buildAnimatedCoverScaleFilter(
	aspectRatio model.AspectRatio,
	duration float64,
	motionIndex int,
) string {
	width, height := aspectRatio.Dimensions(defaultFinalVideoMaxEdge)
	paddedWidth := evenCeil(float64(width) * 1.08)
	paddedHeight := evenCeil(float64(height) * 1.08)
	if paddedWidth <= width {
		paddedWidth = width + 2
	}
	if paddedHeight <= height {
		paddedHeight = height + 2
	}
	if duration <= 0 {
		duration = 3
	}

	xExpr := "0"
	yExpr := "0"
	progressExpr := fmt.Sprintf("min(t/%.3f,1)", duration)
	switch positiveModulo(motionIndex, 4) {
	case 0:
		xExpr = fmt.Sprintf("(in_w-out_w)*%s", progressExpr)
	case 1:
		xExpr = fmt.Sprintf("(in_w-out_w)*(1-%s)", progressExpr)
	case 2:
		yExpr = fmt.Sprintf("(in_h-out_h)*%s", progressExpr)
	default:
		yExpr = fmt.Sprintf("(in_h-out_h)*(1-%s)", progressExpr)
	}

	return fmt.Sprintf(
		"scale=w='ceil(max(%d/iw\\,%d/ih)*iw/2)*2':h='ceil(max(%d/iw\\,%d/ih)*ih/2)*2',crop=%d:%d:x='%s':y='%s',setsar=1,fps=%d,format=yuv420p",
		paddedWidth,
		paddedHeight,
		paddedWidth,
		paddedHeight,
		width,
		height,
		xExpr,
		yExpr,
		defaultFinalVideoFPS,
	)
}

func evenCeil(value float64) int {
	rounded := int(math.Ceil(value))
	if rounded%2 != 0 {
		rounded++
	}
	return rounded
}

func positiveModulo(value int, base int) int {
	if base <= 0 {
		return 0
	}
	result := value % base
	if result < 0 {
		result += base
	}
	return result
}
