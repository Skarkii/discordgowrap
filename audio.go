package discordgowrap

import (
	"log"
	"os/exec"
	"strconv"
)

const (
	channels  int = 2                   // 1 for mono, 2 for stereo
	frameRate int = 48000               // audio sampling rate
	frameSize int = 960                 // uint16 size of each audio frame
	maxBytes  int = (frameSize * 2) * 2 // max size of opus data
)

func PlayAudioFile(v voiceConnection, filename string) {
	run := exec.Command("ffmpeg", "-i", filename, "-f", "s16le", "-ar", strconv.Itoa(frameRate), "-ac", strconv.Itoa(channels), "pipe:1")
	defer run.Process.Kill()
	_, err := run.StdoutPipe()
	if err != nil {
		log.Printf("Error creating pipe for ffmpeg: %v\n", err)
		return
	}

	v.SetSpeaking(true)
	defer v.SetSpeaking(false)

}
