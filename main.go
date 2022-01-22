package main

import (
	"fmt"
	"log"

	"github.com/3d0c/gmf"
)

func main() {
	/// input
	mic, err := gmf.NewInputCtxWithFormatName("default", "alsa")
	if err != nil {
		log.Fatalf("Could not open input context: %s", err)
	}
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

	codec, err := gmf.FindEncoder("libmp3lame")
	if err != nil {
		log.Fatal("find encoder error:", err.Error())
	}

	audioEncCtx := gmf.NewCodecCtx(codec)
	if audioEncCtx == nil {
		log.Fatal("new output codec context error:", err.Error())
	}
	defer audioEncCtx.Free()

	outputCtx, err := gmf.NewOutputCtx("test.mp3")
	if err != nil {
		log.Fatal("new output fail", err.Error())
		return
	}
	defer outputCtx.Free()

	audioEncCtx.SetSampleFmt(gmf.AV_SAMPLE_FMT_S16P).
		SetSampleRate(cc.SampleRate()).
		SetChannels(cc.Channels()).
		SetBitRate(128e3)

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

	/// resample
	options := []*gmf.Option{
		{Key: "in_channel_count", Val: cc.Channels()},
		{Key: "out_channel_count", Val: cc.Channels()},
		{Key: "in_sample_rate", Val: cc.SampleRate()},
		{Key: "out_sample_rate", Val: cc.SampleRate()},
		{Key: "in_sample_fmt", Val: cc.SampleFmt()},
		{Key: "out_sample_fmt", Val: gmf.AV_SAMPLE_FMT_S16P},
	}

	swrCtx, err := gmf.NewSwrCtx(options, audioStream.CodecCtx().Channels(), audioStream.CodecCtx().SampleFmt())
	if err != nil {
		log.Fatal("new swr context error:", err.Error())
	}
	if swrCtx == nil {
		log.Fatal("unable to create Swr Context")
	}

	outputCtx.SetStartTime(0)

	if err := outputCtx.WriteHeader(); err != nil {
		log.Fatal(err.Error())
	}

	outputCtx.Dump()

	count := 0
	for packet := range mic.GetNewPackets() {
		srcFrames, err := cc.Decode(packet)
		packet.Free()
		if err != nil {
			log.Println("capture audio error:", err)
			continue
		}

		exit := false
		for _, srcFrame := range srcFrames {
			wrote := fifo.Write(srcFrame)
			count += wrote

			for fifo.SamplesToRead() >= 1152 {
				winFrame := fifo.Read(1152)
				dstFrame, err := swrCtx.Convert(winFrame)
				if err != nil {
					log.Println("convert audio error:", err)
					exit = true
					break
				}
				if dstFrame == nil {
					continue
				}
				winFrame.Free()

				writePacket, err := dstFrame.Encode(audioEncCtx)
				if err != nil {
					log.Fatal(err)
				}
				if writePacket == nil {
					continue
				}

				if err := outputCtx.WritePacket(writePacket); err != nil {
					log.Println("write packet err", err.Error())
				}
				writePacket.Free()
				dstFrame.Free()
				if count > int(cc.SampleRate())*10 {
					break
				}
			}
		}
		if exit {
			break
		}
	}
}
