package server

import (
	"encoding/binary"
	"math"
)

// trimWAVSilence    PCM WAV firsttail audioseg(ASR before  ize). 
// useuser"by   "  audio  firsttail  0.3~1.5s  audio(by   ,     ), 
//   afteraudiofreqchange  ->     changefast, endtoend    under (  ->outchar). 
//
//  keep 8k/16k/48k, Int16/Float32, 1/2 voice (     inas 16k Int16  voice , 
//     useresolve ;   keep  formorigkindreturnback,     on ). 
//  value:     at threshold  as audio(Int16 use 600/32768~=-35dB, Float32 use 0.02). 
func trimWAVSilence(wav []byte) []byte {
	//     : RIFF head 12B + fmt chunk >=24B + data chunk >=8B
	if len(wav) < 44 {
		return wav
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return wav
	}
	// resolve  fmt and data chunk( edmiddle  store   LIST/bext etc chunk)
	sampleRate := 0
	bitsPerSample := 0
	channels := 0
	var dataStart, dataLen int = -1, 0
	off := 12
	for off+8 <= len(wav) {
		id := string(wav[off : off+4])
		sz := int(binary.LittleEndian.Uint32(wav[off+4 : off+8]))
		if id == "fmt " && sz >= 16 && off+24 <= len(wav) {
			channels = int(binary.LittleEndian.Uint16(wav[off+8 : off+10]))
			sampleRate = int(binary.LittleEndian.Uint32(wav[off+12 : off+16]))
			bitsPerSample = int(binary.LittleEndian.Uint16(wav[off+22 : off+24]))
		}
		if id == "data" {
			dataStart = off + 8
			dataLen = sz
			break
		}
		off += 8 + sz + (sz & 1) // chunk to ( numpatch 1 charnode)
	}
	if dataStart < 0 || dataLen <= 0 || sampleRate <= 0 || bitsPerSample <= 0 || channels <= 0 {
		return wav
	}
	bytesPerSample := bitsPerSample / 8
	if bytesPerSample < 1 || bytesPerSample > 4 {
		return wav
	}
	end := dataStart + dataLen
	if end > len(wav) {
		end = len(wav)
	}
	frameBytes := bytesPerSample * channels
	if frameBytes <= 0 {
		return wav
	}
	frames := (end - dataStart) / frameBytes
	if frames < sampleRate/8 { //  at 0.125s    (  pipe  langaudio  )
		return wav
	}

	isFloat := bitsPerSample == 32
	frameEnergy := func(i int) float64 {
		base := dataStart + i*frameBytes
		e := 0.0
		for c := 0; c < channels; c++ {
			s := 0.0
			if isFloat {
				s = float64(math.Float32frombits(binary.LittleEndian.Uint32(wav[base+c*4 : base+c*4+4])))
			} else {
				v := int16(binary.LittleEndian.Uint16(wav[base+c*2 : base+c*2+2]))
				s = float64(v) / 32768.0
			}
			e += s * s
		}
		return e / float64(channels)
	}

	threshold := 0.02
	if !isFloat {
		threshold = 600.0 / 32768.0
		threshold = threshold * threshold
	}
	// beforeend:  ed audio 
	start := 0
	for start < frames && frameEnergy(start) < threshold {
		start++
	}
	// afterend:  ed audio (  )
	endF := frames
	for endF > start && frameEnergy(endF-1) < threshold {
		endF--
	}
	// protect:   keep  0.15s audiofreq
	minFrames := sampleRate / 6
	if endF-start < minFrames {
		return wav // audiofreq  safety audio/  ,  give   disconnect,      
	}
	if start == 0 && endF == frames {
		return wav // no audio  
	}
	// heavy  WAV:  useorighead, onlymodify RIFF   and data   ,   numdata
	newData := wav[dataStart+start*frameBytes : dataStart+endF*frameBytes]
	out := make([]byte, 0, dataStart+len(newData))
	out = append(out, wav[:dataStart]...)
	out = append(out, newData...)
	// data chunk   
	binary.LittleEndian.PutUint32(out[dataStart-4:dataStart], uint32(len(newData)))
	// RIFF    = file   - 8
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out
}
