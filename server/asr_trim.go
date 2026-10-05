package server

import (
	"encoding/binary"
	"math"
)

// trimWAVSilence 裁剪 PCM WAV 首尾静音段（ASR 前置优化）。
// 用户"按住说话"的录音通常首尾带 0.3~1.5s 静音（按住延迟、松手延迟），
// 裁剪后音频更短 → 平台推理更快，端到端延迟显著下降（松手→出字）。
//
// 支持 8k/16k/48k、Int16/Float32、1/2 声道（平台实际输入为 16k Int16 单声道，
// 这里做通用解析；不支持的格式原样返回，绝不破坏上传）。
// 阈值：帧能量低于 threshold 视为静音（Int16 用 600/32768≈-35dB，Float32 用 0.02）。
func trimWAVSilence(wav []byte) []byte {
	// 最小长度：RIFF 头 12B + fmt chunk ≥24B + data chunk ≥8B
	if len(wav) < 44 {
		return wav
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return wav
	}
	// 解析 fmt 与 data chunk（跳过中间可能存在的 LIST/bext 等 chunk）
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
		off += 8 + sz + (sz & 1) // chunk 对齐（奇数补 1 字节）
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
	if frames < sampleRate/8 { // 少于 0.125s 不裁剪（避免把短促语音裁没）
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
	// 前端：跳过静音帧
	start := 0
	for start < frames && frameEnergy(start) < threshold {
		start++
	}
	// 后端：跳过静音帧（倒序）
	endF := frames
	for endF > start && frameEnergy(endF-1) < threshold {
		endF--
	}
	// 保护：至少保留 0.15s 音频
	minFrames := sampleRate / 6
	if endF-start < minFrames {
		return wav // 音频几乎全静音/太短，交给平台判断，不自行裁坏
	}
	if start == 0 && endF == frames {
		return wav // 无静音可裁
	}
	// 重建 WAV：沿用原头，仅改 RIFF 大小与 data 大小、裁剪数据
	newData := wav[dataStart+start*frameBytes : dataStart+endF*frameBytes]
	out := make([]byte, 0, dataStart+len(newData))
	out = append(out, wav[:dataStart]...)
	out = append(out, newData...)
	// data chunk 长度
	binary.LittleEndian.PutUint32(out[dataStart-4:dataStart], uint32(len(newData)))
	// RIFF 总长 = 文件总长 - 8
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out
}
