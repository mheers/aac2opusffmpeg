package main

import (
	"log"

	. "github.com/3d0c/gmf"
)

func main() {
	/// input
	mic, _ := NewInputCtxWithFormatName("default", "alsa")
	mic.Dump()

	ast, err := mic.GetBestStream(AVMEDIA_TYPE_AUDIO)
	if err != nil {
		log.Fatal("failed to find audio stream")
	}
	cc := ast.CodecCtx()

	/// fifo
	fifo := NewAVAudioFifo(cc.SampleFmt(), cc.Channels(), 1024)
	if fifo == nil {
		log.Fatal("failed to create audio fifo")
	}

	/// output
	codecName := ""
	switch cc.SampleFmt() {
	case AV_SAMPLE_FMT_S16:
		codecName = "pcm_s16le"
	default:
		log.Fatal("sample format not support")
	}
	codec, err := FindEncoder(codecName)
	if err != nil {
		log.Fatal("find encoder error:", err.Error())
	}

	occ := NewCodecCtx(codec)
	if occ == nil {
		log.Fatal("new output codec context error:", err.Error())
	}
	defer Release(occ)

	occ.SetSampleFmt(cc.SampleFmt()).
		SetSampleRate(cc.SampleRate()).
		SetChannels(cc.Channels())

	if err := occ.Open(nil); err != nil {
		log.Fatal("can't open output codec context", err.Error())
		return
	}
	outputCtx, err := NewOutputCtx("test.wav")
	if err != nil {
		log.Fatal("new output fail", err.Error())
		return
	}

	ost := outputCtx.NewStream(codec)
	if ost == nil {
		log.Fatal("Unable to create stream for [%s]\n", codec.LongName())
	}
	defer Release(ost)

	ost.SetCodecCtx(occ)

	if err := outputCtx.WriteHeader(); err != nil {
		log.Fatal(err.Error())
	}

	count := 0
	for packet := range mic.GetNewPackets() {
		srcFrames, err := ast.CodecCtx().Decode(packet)
		packet.Free()
		if err != nil {
			log.Println("capture audio error:", err)
			continue
		}

		for _, srcFrame := range srcFrames {
			wrote := fifo.Write(srcFrame)
			count += wrote

			for fifo.SamplesToRead() >= 1152 {
				dstFrame := fifo.Read(1152)
				if dstFrame == nil {
					continue
				}

				writePacket, err := dstFrame.Encode(occ)
				if err == nil {
					if err := outputCtx.WritePacket(writePacket); err != nil {
						log.Println("write packet err", err.Error())
					}

					writePacket.Free()
				} else {
					log.Fatal(err)
				}
				dstFrame.Free()
			}
			if count > int(cc.SampleRate())*10 {
				break
			}
		}
	}
}
