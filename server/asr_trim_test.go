package server

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func mkWAV16(sr, secs int, voiceStart, voiceEnd float64) []byte {
	var data bytes.Buffer
	for i := 0; i < sr*secs; i++ {
		t := float64(i) / float64(sr)
		var v int16
		if t >= voiceStart && t < voiceEnd {
			v = int16(0.3 * 32768 * math.Sin(2*math.Pi*440*t))
		} else {
			v = int16(0.0005 * 32768 * math.Sin(2*math.Pi*440*t))
		}
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], uint16(v))
		data.Write(b[:])
	}
	pcm := data.Bytes()
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	var sz [4]byte
	binary.LittleEndian.PutUint32(sz[:], uint32(36+len(pcm)))
	wav.Write(sz[:])
	wav.WriteString("WAVEfmt ")
	binary.LittleEndian.PutUint32(sz[:], 16)
	wav.Write(sz[:])
	binary.LittleEndian.PutUint16(sz[:2], 1)
	wav.Write(sz[:2])
	binary.LittleEndian.PutUint16(sz[:2], 1)
	wav.Write(sz[:2])
	binary.LittleEndian.PutUint32(sz[:], uint32(sr))
	wav.Write(sz[:])
	binary.LittleEndian.PutUint32(sz[:], uint32(sr*2))
	wav.Write(sz[:])
	binary.LittleEndian.PutUint16(sz[:2], 2) // blockAlign = 1ch * 2B
	wav.Write(sz[:2])
	binary.LittleEndian.PutUint16(sz[:2], 16) // bitsPerSample
	wav.Write(sz[:2])
	wav.WriteString("data")
	binary.LittleEndian.PutUint32(sz[:], uint32(len(pcm)))
	wav.Write(sz[:])
	wav.Write(pcm)
	return wav.Bytes()
}

func wavDataLen(wav []byte) int {
	off := 12
	for off+8 <= len(wav) {
		id := string(wav[off : off+4])
		sz := int(binary.LittleEndian.Uint32(wav[off+4 : off+8]))
		if id == "data" {
			return sz
		}
		off += 8 + sz + (sz & 1)
	}
	return 0
}

func TestTrimWAVSilence(t *testing.T) {
	// 1s 音频：0.3-0.7s 语音，前后 0.3s 静音 → 应裁到约 0.4s
	wav := mkWAV16(16000, 1, 0.3, 0.7)
	before := wavDataLen(wav)
	out := trimWAVSilence(wav)
	after := wavDataLen(out)
	if after <= 0 || after >= before {
		t.Fatalf("trim 无效: before=%d after=%d", before, after)
	}
	// 期望裁剪后 ~0.4s = 6400B（±0.1s 容差）
	exp := int(16000 * 0.4 * 2)
	lo, hi := exp-3200, exp+3200
	if after < lo || after > hi {
		t.Fatalf("trim 结果异常: after=%d want≈%d", after, exp)
	}
	// RIFF 大小与文件一致
	if got := int(binary.LittleEndian.Uint32(out[4:8])); got != len(out)-8 {
		t.Fatalf("RIFF 大小错误: %d != %d", got, len(out)-8)
	}
	// data chunk 大小字段与真实数据一致（44 字节处是 data 长度字段）
	if int(binary.LittleEndian.Uint32(out[40:44])) != after {
		t.Fatalf("data chunk 大小字段与数据不一致: %d != %d",
			int(binary.LittleEndian.Uint32(out[40:44])), after)
	}
	// 全静音音频：不裁剪（原样返回）
	silent := mkWAV16(16000, 1, 0, 0) // voiceStart==voiceEnd → 全静音
	s2 := trimWAVSilence(silent)
	if wavDataLen(s2) != wavDataLen(silent) {
		t.Fatalf("全静音不应裁剪: %d != %d", wavDataLen(s2), wavDataLen(silent))
	}
	// 垃圾数据：原样返回
	if got := trimWAVSilence([]byte("short")); string(got) != "short" {
		t.Fatalf("垃圾数据不应处理")
	}
}
