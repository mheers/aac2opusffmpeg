package main

import (
	"fmt"
	"log"

	"github.com/3d0c/gmf"
)

func main() {
	/// input
	mic, _ := gmf.NewInputCtxWithFormatName("default", "alsa")
	mic.Dump()

	ast, err := mic.GetBestStream(gmf.AVMEDIA_TYPE_AUDIO)
	if err != nil {
		log.Fatal("failed to find audio stream")
	}
	cc := ast.CodecCtx()

	/// fifo
	fifo := gmf.NewAVAudioFifo(cc.SampleFmt(), cc.Channels(), 1024)
	if fifo == nil {
		log.Fatal("failed to create audio fifo")
	}

	/// output
	codecName := ""
	switch cc.SampleFmt() {
	case gmf.AV_SAMPLE_FMT_S16:
		codecName = "pcm_s16le"
	default:
		log.Fatal("sample format not support")
	}
	codec, err := gmf.FindEncoder(codecName)
	if err != nil {
		log.Fatal("find encoder error:", err.Error())
	}

	audioEncCtx := gmf.NewCodecCtx(codec)
	if audioEncCtx == nil {
		log.Fatal("new output codec context error:", err.Error())
	}
	defer audioEncCtx.Free()

	outputCtx, err := gmf.NewOutputCtx("test.wav")
	if err != nil {
		log.Fatal("new output fail", err.Error())
		return
	}
	defer outputCtx.Free()

	audioEncCtx.SetSampleFmt(cc.SampleFmt()).
		SetSampleRate(cc.SampleRate()).
		SetChannels(cc.Channels())

	if outputCtx.IsGlobalHeader() {
		audioEncCtx.SetFlag(gmf.CODEC_FLAG_GLOBAL_HEADER)
	}

	audioStream := outputCtx.NewStream(codec)
	if audioStream == nil {
		log.Fatal(fmt.Errorf("unable to create stream for audioEnc [%s]", codec.LongName()))
	}
	defer audioStream.Free()

	if err := audioEncCtx.Open(nil); err != nil {
		log.Fatal("can't open output codec context", err.Error())
		return
	}
	audioStream.DumpContexCodec(audioEncCtx)

	outputCtx.SetStartTime(0)

	if err := outputCtx.WriteHeader(); err != nil {
		log.Fatal(err.Error())
	}

	outputCtx.Dump()

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

				writePacket, err := dstFrame.Encode(audioEncCtx)
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
