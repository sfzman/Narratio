package tts

import "testing"

func TestMergeSentenceWAVsNormalizesSampleRateTo24000(t *testing.T) {
	t.Parallel()

	inputPCM := make([]byte, 22050)
	inputWAV, err := buildPCM16WAV(inputPCM, 22050, 1)
	if err != nil {
		t.Fatalf("buildPCM16WAV() error = %v", err)
	}

	merged, duration, err := mergeSentenceWAVs([][]byte{inputWAV}, 0)
	if err != nil {
		t.Fatalf("mergeSentenceWAVs() error = %v", err)
	}
	if duration < 0.49 || duration > 0.51 {
		t.Fatalf("duration = %f, want about 0.5", duration)
	}

	audio, err := decodeWAV(merged)
	if err != nil {
		t.Fatalf("decodeWAV() error = %v", err)
	}
	if audio.SampleRate != wavSampleRate {
		t.Fatalf("audio.SampleRate = %d, want %d", audio.SampleRate, wavSampleRate)
	}
	if audio.ChannelCount != wavChannelCount {
		t.Fatalf("audio.ChannelCount = %d, want %d", audio.ChannelCount, wavChannelCount)
	}
	if audio.BitsPerSample != wavBitsPerSample {
		t.Fatalf("audio.BitsPerSample = %d, want %d", audio.BitsPerSample, wavBitsPerSample)
	}
}
