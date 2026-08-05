export interface LivePCMChunk {
  sequence: number;
  startMs: number;
  endMs: number;
  format: "wav" | "linear16";
  sampleRate: number;
  data: ArrayBuffer;
}

interface WorkletMessage {
  type: "samples" | "flushed";
  samples?: ArrayBuffer;
}

const DEFAULT_CHUNK_DURATION_MS = 4_000;
const MINIMUM_WAV_FLUSH_DURATION_MS = 250;
const MINIMUM_STREAM_FLUSH_DURATION_MS = 20;

export class LivePCMStream {
  private context?: AudioContext;
  private source?: MediaStreamAudioSourceNode;
  private worklet?: AudioWorkletNode;
  private silentSink?: GainNode;
  private sampleRate = 48_000;
  private targetSamples = 0;
  private queuedSamples: Float32Array[] = [];
  private queuedSampleCount = 0;
  private emittedSampleCount = 0;
  private sequence = 0;
  private flushResolver?: () => void;
  private flushTimer?: number;

  constructor(
    private readonly onChunk: (chunk: LivePCMChunk) => void,
    private readonly chunkDurationMs = DEFAULT_CHUNK_DURATION_MS,
    private readonly format: LivePCMChunk["format"] = "wav",
  ) {}

  async start(stream: MediaStream): Promise<void> {
    const context = new AudioContext({ latencyHint: "interactive", sampleRate: 48_000 });
    this.context = context;
    this.sampleRate = context.sampleRate;
    this.targetSamples = Math.max(1, Math.round((this.sampleRate * this.chunkDurationMs) / 1_000));
    await context.audioWorklet.addModule("/pcm-capture-worklet.js");

    const source = context.createMediaStreamSource(stream);
    const worklet = new AudioWorkletNode(context, "writers-studio-pcm-capture", {
      numberOfInputs: 1,
      numberOfOutputs: 1,
      outputChannelCount: [1],
    });
    const silentSink = context.createGain();
    silentSink.gain.value = 0;
    worklet.port.onmessage = (event: MessageEvent<WorkletMessage>) => {
      if (event.data.type === "flushed") {
        this.resolveWorkletFlush();
        return;
      }
      if (event.data.samples) {
        this.pushSamples(new Float32Array(event.data.samples));
      }
    };
    source.connect(worklet);
    worklet.connect(silentSink);
    silentSink.connect(context.destination);
    this.source = source;
    this.worklet = worklet;
    this.silentSink = silentSink;
    if (context.state === "suspended") {
      await context.resume();
    }
  }

  async pause(): Promise<void> {
    await this.flush();
    if (this.context?.state === "running") {
      await this.context.suspend();
    }
  }

  async resume(): Promise<void> {
    if (this.context?.state === "suspended") {
      await this.context.resume();
    }
  }

  async stop(): Promise<void> {
    await this.flush();
    this.source?.disconnect();
    this.worklet?.disconnect();
    this.silentSink?.disconnect();
    this.worklet = undefined;
    this.source = undefined;
    this.silentSink = undefined;
    if (this.context && this.context.state !== "closed") {
      await this.context.close();
    }
    this.context = undefined;
  }

  private async flush(): Promise<void> {
    if (this.worklet && this.context?.state === "running") {
      await new Promise<void>((resolve) => {
        this.flushResolver = resolve;
        this.flushTimer = window.setTimeout(() => this.resolveWorkletFlush(), 500);
        this.worklet?.port.postMessage({ type: "flush" });
      });
    }
    const minimumDurationMs =
      this.format === "linear16" ? MINIMUM_STREAM_FLUSH_DURATION_MS : MINIMUM_WAV_FLUSH_DURATION_MS;
    const minimumSamples = Math.round((this.sampleRate * minimumDurationMs) / 1_000);
    if (this.queuedSampleCount >= minimumSamples) {
      this.emitSamples(this.takeSamples(this.queuedSampleCount));
    } else {
      this.queuedSamples = [];
      this.queuedSampleCount = 0;
    }
  }

  private resolveWorkletFlush(): void {
    window.clearTimeout(this.flushTimer);
    this.flushTimer = undefined;
    this.flushResolver?.();
    this.flushResolver = undefined;
  }

  private pushSamples(samples: Float32Array): void {
    if (samples.length === 0) {
      return;
    }
    this.queuedSamples.push(samples);
    this.queuedSampleCount += samples.length;
    while (this.queuedSampleCount >= this.targetSamples) {
      this.emitSamples(this.takeSamples(this.targetSamples));
    }
  }

  private takeSamples(count: number): Float32Array {
    const output = new Float32Array(count);
    let outputOffset = 0;
    while (outputOffset < count) {
      const head = this.queuedSamples[0];
      if (!head) {
        break;
      }
      const readable = Math.min(head.length, count - outputOffset);
      output.set(head.subarray(0, readable), outputOffset);
      outputOffset += readable;
      if (readable === head.length) {
        this.queuedSamples.shift();
      } else {
        this.queuedSamples[0] = head.subarray(readable);
      }
      this.queuedSampleCount -= readable;
    }
    return output;
  }

  private emitSamples(samples: Float32Array): void {
    const startSample = this.emittedSampleCount;
    this.emittedSampleCount += samples.length;
    const startMs = Math.round((startSample * 1_000) / this.sampleRate);
    const endMs = Math.round((this.emittedSampleCount * 1_000) / this.sampleRate);
    this.onChunk({
      sequence: this.sequence++,
      startMs,
      endMs,
      format: this.format,
      sampleRate: this.sampleRate,
      data:
        this.format === "linear16"
          ? encodePCM16(samples)
          : encodePCM16WAV(samples, this.sampleRate),
    });
  }
}

export function encodePCM16(samples: Float32Array): ArrayBuffer {
  const buffer = new ArrayBuffer(samples.length * 2);
  const view = new DataView(buffer);
  for (let index = 0; index < samples.length; index += 1) {
    const sample = Math.max(-1, Math.min(1, samples[index] ?? 0));
    view.setInt16(index * 2, sample < 0 ? sample * 0x8000 : sample * 0x7fff, true);
  }
  return buffer;
}

export function encodePCM16WAV(samples: Float32Array, sampleRate: number): ArrayBuffer {
  const bytesPerSample = 2;
  const headerSize = 44;
  const dataSize = samples.length * bytesPerSample;
  const buffer = new ArrayBuffer(headerSize + dataSize);
  const view = new DataView(buffer);
  writeASCII(view, 0, "RIFF");
  view.setUint32(4, 36 + dataSize, true);
  writeASCII(view, 8, "WAVE");
  writeASCII(view, 12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * bytesPerSample, true);
  view.setUint16(32, bytesPerSample, true);
  view.setUint16(34, 16, true);
  writeASCII(view, 36, "data");
  view.setUint32(40, dataSize, true);
  new Uint8Array(buffer, headerSize).set(new Uint8Array(encodePCM16(samples)));
  return buffer;
}

function writeASCII(view: DataView, offset: number, value: string): void {
  for (let index = 0; index < value.length; index += 1) {
    view.setUint8(offset + index, value.charCodeAt(index));
  }
}
