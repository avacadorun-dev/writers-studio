class WritersStudioPCMProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.buffer = new Float32Array(4096);
    this.offset = 0;
    this.port.onmessage = (event) => {
      if (event.data?.type === "flush") {
        this.flush();
        this.port.postMessage({ type: "flushed" });
      }
    };
  }

  process(inputs) {
    const channels = inputs[0];
    if (!channels || channels.length === 0 || channels[0].length === 0) {
      return true;
    }

    const frameLength = channels[0].length;
    let frameOffset = 0;
    while (frameOffset < frameLength) {
      const writable = Math.min(this.buffer.length - this.offset, frameLength - frameOffset);
      for (let index = 0; index < writable; index += 1) {
        let sample = 0;
        for (const channel of channels) {
          sample += channel[frameOffset + index] ?? 0;
        }
        this.buffer[this.offset + index] = sample / channels.length;
      }
      this.offset += writable;
      frameOffset += writable;
      if (this.offset === this.buffer.length) {
        this.flush();
      }
    }
    return true;
  }

  flush() {
    if (this.offset === 0) {
      return;
    }
    const samples = this.buffer.slice(0, this.offset);
    this.port.postMessage({ type: "samples", samples: samples.buffer }, [samples.buffer]);
    this.offset = 0;
  }
}

registerProcessor("writers-studio-pcm-capture", WritersStudioPCMProcessor);
