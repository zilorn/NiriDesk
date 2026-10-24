import type { AudioState } from "../types";

const MAGIC = "NDA1";
const HEADER_SIZE = 24;
const FORMAT_S16LE = 1;
const MIN_LEAD_SECONDS = 0.01;
const MAX_SCHEDULE_AHEAD_SECONDS = 1;

type StateListener = (state: AudioState, detail?: string) => void;

interface FrameHeader {
  channels: number;
  sampleRate: number;
  timestamp: number;
}

export class RemoteAudio {
  private socket: WebSocket | null = null;
  private context: AudioContext | null = null;
  private sources = new Set<AudioBufferSourceNode>();
  private generation = 0;
  private delaySeconds: number;
  private baseTimestamp = 0;
  private baseTime = 0;
  private lateDrops = 0;
  private state: AudioState = "idle";
  private readonly onState: StateListener;

  constructor(delayMs: number, onState: StateListener) {
    this.delaySeconds = Math.max(0.04, delayMs / 1000);
    this.onState = onState;
  }

  async start(): Promise<void> {
    this.stop(true);
    const generation = ++this.generation;
    this.setState("starting");

    let context: AudioContext;
    try {
      context = new AudioContext({ latencyHint: "interactive" });
      this.context = context;
      if (context.state === "suspended") await context.resume();
    } catch {
      this.closeTransport();
      this.setState("error", "浏览器阻止了音频播放");
      throw new Error("浏览器阻止了音频播放，请再次点按声音按钮");
    }

    try {
      const scheme = window.location.protocol === "https:" ? "wss" : "ws";
      const socket = new WebSocket(`${scheme}://${window.location.host}/audio`);
      socket.binaryType = "arraybuffer";
      this.socket = socket;

      await new Promise<void>((resolve, reject) => {
        let opened = false;
        socket.onopen = () => {
          opened = true;
          resolve();
        };
        socket.onmessage = (event) => {
          if (generation !== this.generation) return;
          if (event.data instanceof ArrayBuffer) this.handleFrame(event.data);
        };
        socket.onerror = () => {
          if (!opened) reject(new Error("无法连接音频通道"));
        };
        socket.onclose = (event) => {
          if (generation !== this.generation) return;
          const detail = event.reason || `音频连接已断开（${event.code}）`;
          this.closeTransport();
          this.generation++;
          this.setState("error", detail);
          if (!opened) reject(new Error(detail));
        };
      });
    } catch (error) {
      if (generation === this.generation) {
        this.closeTransport();
        this.generation++;
      }
      const message = error instanceof Error ? error.message : "无法启动音频";
      this.setState("error", message);
      throw error;
    }
  }

  stop(resetState = false): void {
    this.generation++;
    this.closeTransport();
    if (resetState || this.state !== "idle") this.setState("idle");
  }

  setDelay(delayMs: number): void {
    const next = Math.max(40, Math.min(500, Math.round(delayMs))) / 1000;
    if (Math.abs(next - this.delaySeconds) < 0.001) return;
    this.delaySeconds = next;
    this.stopSources();
    this.baseTimestamp = 0;
    this.baseTime = 0;
    this.lateDrops = 0;
  }

  private closeTransport(): void {
    const socket = this.socket;
    this.socket = null;
    if (socket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onerror = null;
      socket.onclose = null;
      socket.close();
    }
    this.stopSources();
    const context = this.context;
    this.context = null;
    if (context) void context.close().catch(() => undefined);
    this.baseTimestamp = 0;
    this.baseTime = 0;
    this.lateDrops = 0;
  }

  private stopSources(): void {
    for (const source of this.sources) {
      try {
        source.stop();
      } catch {
        // The node may already have ended.
      }
    }
    this.sources.clear();
  }

  private handleFrame(data: ArrayBuffer): void {
    const context = this.context;
    if (!context || data.byteLength <= HEADER_SIZE) return;
    if (String.fromCharCode(...new Uint8Array(data, 0, 4)) !== MAGIC) return;

    const view = new DataView(data);
    if (view.getUint8(4) !== 1 || view.getUint8(5) !== FORMAT_S16LE) return;
    const header: FrameHeader = {
      channels: view.getUint16(6, true),
      sampleRate: view.getUint32(8, true),
      timestamp: Number(view.getBigUint64(12, true)),
    };
    if (header.channels !== 2 || header.sampleRate !== 48000) return;
    if (context.state !== "running") {
      void context.resume().catch(() => undefined);
      this.stopSources();
      this.baseTimestamp = 0;
      this.baseTime = 0;
      this.lateDrops = 0;
      return;
    }

    const frameBytes = header.channels * 2;
    const frames = (data.byteLength - HEADER_SIZE) / frameBytes;
    if (!Number.isInteger(frames) || frames <= 0) return;

    const pcm = new Int16Array(data, HEADER_SIZE, frames * header.channels);
    const buffer = context.createBuffer(header.channels, frames, header.sampleRate);
    for (let channel = 0; channel < header.channels; channel++) {
      const output = buffer.getChannelData(channel);
      for (let frame = 0; frame < frames; frame++) {
        output[frame] = pcm[frame * header.channels + channel] / 32768;
      }
    }

    if (this.baseTimestamp === 0) {
      this.baseTimestamp = header.timestamp;
      this.baseTime = context.currentTime + this.delaySeconds;
    }

    let when = this.baseTime + (header.timestamp - this.baseTimestamp) / 1_000_000;
    if (when > context.currentTime + MAX_SCHEDULE_AHEAD_SECONDS) {
      this.baseTimestamp = header.timestamp;
      this.baseTime = context.currentTime + this.delaySeconds;
      when = this.baseTime;
    }

    if (when < context.currentTime + MIN_LEAD_SECONDS) {
      this.lateDrops++;
      if (this.lateDrops < 3) return;
      this.baseTimestamp = header.timestamp;
      this.baseTime = context.currentTime + this.delaySeconds;
      when = this.baseTime;
    }
    this.lateDrops = 0;

    const source = context.createBufferSource();
    source.buffer = buffer;
    source.connect(context.destination);
    source.onended = () => this.sources.delete(source);
    source.start(when);
    this.sources.add(source);
    if (this.state !== "playing") this.setState("playing");
  }

  private setState(state: AudioState, detail = ""): void {
    this.state = state;
    this.onState(state, detail);
  }
}
