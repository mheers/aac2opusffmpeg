package main

import (
	"fmt"
	"log"

	"github.com/3d0c/gmf"
)

func main() {
	/// input
	audioInput, err := gmf.NewInputCtx("demofiles/test.aac")
	if err != nil {
		log.Fatalf("Could not open input context: %s", err)
	}
	audioInput.Dump()

	ast, err := audioInput.GetBestStream(gmf.AVMEDIA_TYPE_AUDIO)
	if err != nil {
		log.Fatal("failed to find audio stream")
	}
	cc := ast.CodecCtx()

	/// fifo
	fifo := gmf.NewAVAudioFifo(cc.SampleFmt(), cc.Channels(), 64)
	if fifo == nil {
		log.Fatal("failed to create audio fifo")
	}

	codec, err := gmf.FindEncoder("libvorbis")
	if err != nil {
		log.Fatal("find encoder error:", err.Error())
	}

	audioEncCtx := gmf.NewCodecCtx(codec)
	if audioEncCtx == nil {
		log.Fatal("new output codec context error:", err.Error())
	}
	defer audioEncCtx.Free()

	outputCtx, err := gmf.NewOutputCtx("test.ogg")
	if err != nil {
		log.Fatal("new output fail", err.Error())
		return
	}
	defer outputCtx.Free()

	outSampleFormat := gmf.AV_SAMPLE_FMT_FLTP

	audioEncCtx.SetSampleFmt(outSampleFormat).
		SetSampleRate(cc.SampleRate()).
		SetChannels(cc.Channels()).
		SetBitRate(128000)

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
		{Key: "out_sample_rate", Val: 48000},
		{Key: "in_sample_fmt", Val: cc.SampleFmt()},
		{Key: "out_sample_fmt", Val: audioStream.CodecCtx().SampleFmt()},
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

	writeCount := 0
	count := 0
	packetIndex := 0
	for packet := range audioInput.GetNewPackets() {
		packetIndex++
		srcFrames, err := cc.Decode(packet)
		packet.Free()
		if err != nil {
			log.Println("capture audio error:", err)
			continue
		}

		exit := false
		for _, srcFrame := range srcFrames {
			wrote := fifo.Write(srcFrame)
			writeCount += wrote

			frameSize := audioEncCtx.FrameSize() // for ogg 64 is the maximum
			for fifo.SamplesToRead() >= frameSize {
				winFrame := fifo.Read(frameSize)
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

				count += frameSize
				dstFrame.SetPts(int64(count))

				writePacket, err := dstFrame.Encode(audioEncCtx)
				if err != nil {
					log.Fatal(err)
				}
				if writePacket == nil {
					continue
				}
				writePacket.SetStreamIndex(audioStream.Index())

				if err := outputCtx.WritePacket(writePacket); err != nil {
					log.Println("write packet err", err.Error())
				}
				writePacket.Free()
				dstFrame.Free()
				if writeCount > int(cc.SampleRate())*1152 {
					break
				}
			}
		}
		if exit {
			break
		}
	}
}
