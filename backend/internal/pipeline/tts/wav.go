package tts

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	wavSampleRate             = 24000
	wavChannelCount           = 1
	wavBitsPerSample          = 16
	defaultSentenceGapSeconds = 0.1
	maxPCM16                  = 32767
	minPCM16                  = -32768
)

type audioFile struct {
	Path string
	Data []byte
}

type wavAudio struct {
	SampleRate    int
	ChannelCount  int
	BitsPerSample int
	PCM           []byte
}

func buildPlaceholderAudioFiles(segments []AudioSegment) []audioFile {
	files := make([]audioFile, 0, len(segments))
	for _, segment := range segments {
		data, err := buildSilentWAV(segment.Duration)
		if err != nil {
			continue
		}
		files = append(files, audioFile{
			Path: segment.FilePath,
			Data: data,
		})
	}

	return files
}

func writeAudioFiles(artifacts artifactWriter, files []audioFile) error {
	for _, file := range files {
		if err := artifacts.WriteBytes(file.Path, file.Data); err != nil {
			return fmt.Errorf("write audio file: %w", err)
		}
	}

	return nil
}

func buildSilentWAV(durationSeconds float64) ([]byte, error) {
	sampleCount := int(durationSeconds*wavSampleRate + 0.5)
	if sampleCount <= 0 {
		sampleCount = wavSampleRate
	}

	return buildPCM16MonoWAV(make([]byte, sampleCount*wavBlockAlign()))
}

func buildPCM16MonoWAV(pcm []byte) ([]byte, error) {
	return buildPCM16WAV(pcm, wavSampleRate, wavChannelCount)
}

func buildPCM16WAV(pcm []byte, sampleRate int, channelCount int) ([]byte, error) {
	bytesPerSample := wavBitsPerSample / 8
	blockAlign := channelCount * bytesPerSample
	byteRate := sampleRate * blockAlign
	dataSize := len(pcm)
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
	if err := writeChunk(uint16(channelCount)); err != nil {
		return nil, err
	}
	if err := writeChunk(uint32(sampleRate)); err != nil {
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
	if _, err := buffer.Write(pcm); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func mergeSentenceWAVs(sentenceWAVs [][]byte, gapSeconds float64) ([]byte, float64, error) {
	if len(sentenceWAVs) == 0 {
		return nil, 0, fmt.Errorf("no sentence wavs to merge")
	}

	gapPCM := make([]byte, gapSampleCount(gapSeconds)*wavBlockAlign())
	combinedPCM := make([]byte, 0, estimatedPCMSize(sentenceWAVs, len(gapPCM)))
	for index, item := range sentenceWAVs {
		audio, err := decodeWAV(item)
		if err != nil {
			return nil, 0, fmt.Errorf("decode sentence wav %d: %w", index, err)
		}
		audio, err = normalizeWAVForMerge(audio)
		if err != nil {
			return nil, 0, fmt.Errorf("normalize sentence wav %d: %w", index, err)
		}
		combinedPCM = append(combinedPCM, audio.PCM...)
		if len(gapPCM) > 0 && index < len(sentenceWAVs)-1 {
			combinedPCM = append(combinedPCM, gapPCM...)
		}
	}

	data, err := buildPCM16MonoWAV(combinedPCM)
	if err != nil {
		return nil, 0, fmt.Errorf("build merged wav: %w", err)
	}

	return data, pcmDurationSeconds(combinedPCM), nil
}

func decodeWAV(data []byte) (wavAudio, error) {
	if len(data) < 44 {
		return wavAudio{}, fmt.Errorf("wav data too short")
	}
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return wavAudio{}, fmt.Errorf("invalid wav header")
	}

	audio := wavAudio{}
	offset := 12
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8
		if offset+chunkSize > len(data) {
			return wavAudio{}, fmt.Errorf("wav chunk %s exceeds data length", chunkID)
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return wavAudio{}, fmt.Errorf("wav fmt chunk too short")
			}
			audioFormat := binary.LittleEndian.Uint16(data[offset : offset+2])
			if audioFormat != 1 {
				return wavAudio{}, fmt.Errorf("unsupported wav audio format %d", audioFormat)
			}
			audio.ChannelCount = int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
			audio.SampleRate = int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			audio.BitsPerSample = int(binary.LittleEndian.Uint16(data[offset+14 : offset+16]))
		case "data":
			audio.PCM = append([]byte(nil), data[offset:offset+chunkSize]...)
		}

		offset += chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}

	if audio.SampleRate == 0 || audio.ChannelCount == 0 || audio.BitsPerSample == 0 {
		return wavAudio{}, fmt.Errorf("wav fmt chunk missing")
	}
	if len(audio.PCM) == 0 {
		return wavAudio{}, fmt.Errorf("wav data chunk missing")
	}

	return audio, nil
}

func normalizeWAVForMerge(audio wavAudio) (wavAudio, error) {
	if audio.BitsPerSample != wavBitsPerSample {
		return wavAudio{}, fmt.Errorf("unexpected bits per sample %d", audio.BitsPerSample)
	}
	if audio.ChannelCount <= 0 {
		return wavAudio{}, fmt.Errorf("unexpected channel count %d", audio.ChannelCount)
	}

	monoSamples, err := decodePCM16Mono(audio)
	if err != nil {
		return wavAudio{}, err
	}
	resampled := resamplePCM16(monoSamples, audio.SampleRate, wavSampleRate)

	return wavAudio{
		SampleRate:    wavSampleRate,
		ChannelCount:  wavChannelCount,
		BitsPerSample: wavBitsPerSample,
		PCM:           encodePCM16(resampled),
	}, nil
}

func decodePCM16Mono(audio wavAudio) ([]int16, error) {
	frameSize := audio.ChannelCount * (audio.BitsPerSample / 8)
	if frameSize <= 0 || len(audio.PCM)%frameSize != 0 {
		return nil, fmt.Errorf("invalid pcm data size")
	}

	frameCount := len(audio.PCM) / frameSize
	samples := make([]int16, 0, frameCount)
	for frame := 0; frame < frameCount; frame++ {
		offset := frame * frameSize
		var mixed int32
		for channel := 0; channel < audio.ChannelCount; channel++ {
			channelOffset := offset + channel*2
			mixed += int32(int16(binary.LittleEndian.Uint16(audio.PCM[channelOffset : channelOffset+2])))
		}
		samples = append(samples, int16(mixed/int32(audio.ChannelCount)))
	}

	return samples, nil
}

func resamplePCM16(samples []int16, fromRate int, toRate int) []int16 {
	if len(samples) == 0 || fromRate <= 0 || toRate <= 0 {
		return nil
	}
	if fromRate == toRate {
		return append([]int16(nil), samples...)
	}

	outputLen := int(math.Round(float64(len(samples)) * float64(toRate) / float64(fromRate)))
	if outputLen <= 0 {
		outputLen = 1
	}
	if len(samples) == 1 {
		return repeatSample(samples[0], outputLen)
	}

	resampled := make([]int16, outputLen)
	ratio := float64(fromRate) / float64(toRate)
	for index := range resampled {
		sourcePos := float64(index) * ratio
		left := int(sourcePos)
		if left >= len(samples)-1 {
			resampled[index] = samples[len(samples)-1]
			continue
		}
		fraction := sourcePos - float64(left)
		leftValue := float64(samples[left])
		rightValue := float64(samples[left+1])
		value := leftValue + (rightValue-leftValue)*fraction
		resampled[index] = clampPCM16(value)
	}

	return resampled
}

func encodePCM16(samples []int16) []byte {
	pcm := make([]byte, len(samples)*2)
	for index, sample := range samples {
		binary.LittleEndian.PutUint16(pcm[index*2:index*2+2], uint16(sample))
	}

	return pcm
}

func repeatSample(sample int16, count int) []int16 {
	if count <= 0 {
		return nil
	}

	samples := make([]int16, count)
	for index := range samples {
		samples[index] = sample
	}

	return samples
}

func clampPCM16(value float64) int16 {
	if value > maxPCM16 {
		return maxPCM16
	}
	if value < minPCM16 {
		return minPCM16
	}

	return int16(math.Round(value))
}

func wavBlockAlign() int {
	return wavChannelCount * (wavBitsPerSample / 8)
}

func gapSampleCount(gapSeconds float64) int {
	if gapSeconds <= 0 {
		return 0
	}

	return int(gapSeconds*wavSampleRate + 0.5)
}

func pcmDurationSeconds(pcm []byte) float64 {
	if len(pcm) == 0 {
		return 0
	}

	return float64(len(pcm)) / float64(wavBlockAlign()*wavSampleRate)
}

func estimatedPCMSize(sentenceWAVs [][]byte, gapPCMSize int) int {
	total := 0
	for _, item := range sentenceWAVs {
		total += len(item)
	}
	if len(sentenceWAVs) > 1 {
		total += (len(sentenceWAVs) - 1) * gapPCMSize
	}

	return total
}
