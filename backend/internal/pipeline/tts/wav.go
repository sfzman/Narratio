package tts

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	wavSampleRate    = 24000
	wavChannelCount  = 1
	wavBitsPerSample = 16
)

func writePlaceholderAudioSegments(artifacts artifactWriter, segments []AudioSegment) error {
	for _, segment := range segments {
		data, err := buildSilentWAV(segment.Duration)
		if err != nil {
			return fmt.Errorf("build silent wav: %w", err)
		}
		if err := artifacts.WriteBytes(segment.FilePath, data); err != nil {
			return fmt.Errorf("write audio segment: %w", err)
		}
	}

	return nil
}

func buildSilentWAV(durationSeconds float64) ([]byte, error) {
	sampleCount := int(durationSeconds*wavSampleRate + 0.5)
	if sampleCount <= 0 {
		sampleCount = wavSampleRate
	}

	bytesPerSample := wavBitsPerSample / 8
	blockAlign := wavChannelCount * bytesPerSample
	byteRate := wavSampleRate * blockAlign
	dataSize := sampleCount * blockAlign
	riffSize := 36 + dataSize

	var buffer bytes.Buffer
	writeChunk := func(value any) error {
		return binary.Write(&buffer, binary.LittleEndian, value)
	}

	if _, err := buffer.WriteString("RIFF"); err != nil {
		return nil, err
	}
	if err := writeChunk(uint32(riffSize)); err != nil {
		return nil, err
	}
	if _, err := buffer.WriteString("WAVE"); err != nil {
		return nil, err
	}
	if _, err := buffer.WriteString("fmt "); err != nil {
		return nil, err
	}
	if err := writeChunk(uint32(16)); err != nil {
		return nil, err
	}
	if err := writeChunk(uint16(1)); err != nil {
		return nil, err
	}
	if err := writeChunk(uint16(wavChannelCount)); err != nil {
		return nil, err
	}
	if err := writeChunk(uint32(wavSampleRate)); err != nil {
		return nil, err
	}
	if err := writeChunk(uint32(byteRate)); err != nil {
		return nil, err
	}
	if err := writeChunk(uint16(blockAlign)); err != nil {
		return nil, err
	}
	if err := writeChunk(uint16(wavBitsPerSample)); err != nil {
		return nil, err
	}
	if _, err := buffer.WriteString("data"); err != nil {
		return nil, err
	}
	if err := writeChunk(uint32(dataSize)); err != nil {
		return nil, err
	}
	if _, err := buffer.Write(make([]byte, dataSize)); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
