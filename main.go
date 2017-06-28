package main

// #cgo pkg-config: vorbisfile
// #include <stdlib.h>
// #include <vorbis/vorbisfile.h>
import "C"

import (
	"fmt"
	"io"
	"log"
	"os"

	"unsafe"

	"github.com/jfreymuth/oggvorbis"
	"github.com/oov/audio/wave"
)

func newWaveWriter(w io.Writer, rate int, channels int) (*wave.Writer, error) {
	const (
		BitsPerSample  = 32
		BytesPerSample = BitsPerSample / 8
	)
	return wave.NewWriter(w, &wave.WaveFormatExtensible{
		Format: wave.WaveFormatEx{
			FormatTag:      wave.WAVE_FORMAT_IEEE_FLOAT,
			Channels:       uint16(channels),
			SamplesPerSec:  uint32(rate),
			AvgBytesPerSec: uint32(rate * channels * BytesPerSample),
			BlockAlign:     uint16(channels * BytesPerSample),
			BitsPerSample:  BitsPerSample,
		},
	})
}
func decodeCgo(inName, outName string) error {
	log.Println("cgo:")
	vf := (*C.OggVorbis_File)(C.malloc(C.size_t(unsafe.Sizeof(C.OggVorbis_File{}))))
	defer C.free(unsafe.Pointer(vf))

	inNameC := C.CString(inName)
	defer C.free(unsafe.Pointer(inNameC))
	if r := C.ov_fopen(inNameC, vf); r != 0 {
		return fmt.Errorf("could not open: %d", r)
	}
	defer C.ov_clear(vf)

	vi := C.ov_info(vf, -1)
	if vi == nil {
		return fmt.Errorf("could not get stream info")
	}

	log.Println("  samples:", C.ov_pcm_total(vf, -1))

	out, err := os.Create(outName)
	if err != nil {
		return err
	}
	defer out.Close()

	w, err := newWaveWriter(out, int(vi.rate), int(vi.channels))
	if err != nil {
		return err
	}
	defer w.Close()

	ln := 0
	buf := make([][]float32, vi.channels)
	for {
		var pcm *[1 << 30]*[1 << 30]float32
		var cursec C.int
		samples := C.ov_read_float(vf, (***C.float)(unsafe.Pointer(&pcm)), 1024, &cursec)
		if samples == 0 {
			break
		} else if samples > 0 {
			for ch := range buf {
				buf[ch] = pcm[ch][:samples:samples]
			}
			if _, err = w.WriteFloat32Interleaved(buf); err != nil {
				return err
			}
			ln += int(samples)
		}
	}
	log.Println("  decoded samples:", ln)
	return nil
}

func decodeNative(inName, outName string) error {
	log.Println("native:")
	f, err := os.Open(inName)
	if err != nil {
		return err
	}
	defer f.Close()

	r, err := oggvorbis.NewReader(f)
	if err != nil {
		return err
	}

	log.Println("  samples:", r.Length())

	out, err := os.Create(outName)
	if err != nil {
		return err
	}
	defer out.Close()

	chs := r.Channels()
	w, err := newWaveWriter(out, r.SampleRate(), chs)
	if err != nil {
		return err
	}
	defer w.Close()

	const BufSize = 1024
	tmpbuf := make([]float32, BufSize*chs)
	tmpbuf2 := make([]float32, BufSize*chs)
	buf := make([][]float32, chs)
	ln := 0
	for {
		n, err := r.Read(tmpbuf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		s := 0
		for i := 0; i < n; s++ {
			for ch := 0; ch < chs; ch++ {
				tmpbuf2[ch*BufSize+s] = tmpbuf[i]
				i++
			}
		}
		ln += s
		for ch := range buf {
			buf[ch] = tmpbuf2[ch*BufSize : ch*BufSize+s : ch*BufSize+s]
		}
		if _, err = w.WriteFloat32Interleaved(buf); err != nil {
			return err
		}
	}
	log.Println("  decoded samples:", ln)
	return nil
}

func main() {
	if err := decodeCgo(os.Args[1], "test_cgo.wav"); err != nil {
		log.Fatalln(err)
	}
	if err := decodeNative(os.Args[1], "test_native.wav"); err != nil {
		log.Fatalln(err)
	}
}
