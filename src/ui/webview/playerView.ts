/**
 * Player webview
 * Full player UI with album art and controls
 */

import * as vscode from 'vscode';
import * as path from 'path';
import { IPCClient } from '../../daemon/client';
import type { StatusResponse } from '../../types';

/**
 * Player webview panel manager
 */
export class PlayerWebviewPanel {
  public static currentPanel: PlayerWebviewPanel | undefined;
  private readonly panel: vscode.WebviewPanel;
  private readonly extensionUri: vscode.Uri;
  private client: IPCClient;
  private disposables: vscode.Disposable[] = [];
  private statusInterval: NodeJS.Timeout | null = null;

  public static createOrShow(extensionUri: vscode.Uri, client: IPCClient): PlayerWebviewPanel {
    const column = vscode.window.activeTextEditor?.viewColumn || vscode.ViewColumn.One;

    // If we already have a panel, show it
    if (PlayerWebviewPanel.currentPanel) {
      PlayerWebviewPanel.currentPanel.panel.reveal(column);
      return PlayerWebviewPanel.currentPanel;
    }

    // Otherwise, create a new panel
    const panel = vscode.window.createWebviewPanel(
      'localMediaPlayer',
      'Music Player',
      column,
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        localResourceRoots: [vscode.Uri.joinPath(extensionUri, 'media')],
      }
    );

    PlayerWebviewPanel.currentPanel = new PlayerWebviewPanel(panel, extensionUri, client);
    return PlayerWebviewPanel.currentPanel;
  }

  private constructor(
    panel: vscode.WebviewPanel,
    extensionUri: vscode.Uri,
    client: IPCClient
  ) {
    this.panel = panel;
    this.extensionUri = extensionUri;
    this.client = client;

    // Set the webview's initial html content
    this.update();

    // Listen for when the panel is disposed
    this.panel.onDidDispose(() => this.dispose(), null, this.disposables);

    // Handle messages from the webview
    this.panel.webview.onDidReceiveMessage(
      async (message) => {
        await this.handleMessage(message);
      },
      null,
      this.disposables
    );

    // Start status updates
    this.startStatusUpdates();
  }

  private async handleMessage(message: { command: string; value?: unknown }): Promise<void> {
    try {
      switch (message.command) {
        case 'play':
          await this.client.resume();
          break;
        case 'pause':
          await this.client.pause();
          break;
        case 'next':
          await this.client.next();
          break;
        case 'previous': {
          // If more than 5 seconds into the song, restart it
          // Otherwise, go to the previous track
          const status = await this.client.getStatus();
          if ((status.position || 0) > 5000) {
            await this.client.seek(0);
          } else {
            await this.client.prev();
          }
          break;
        }
        case 'stop':
          await this.client.stop();
          break;
        case 'seek':
          if (typeof message.value === 'number') {
            await this.client.seek(message.value);
          }
          break;
        case 'volume':
          if (typeof message.value === 'number') {
            await this.client.setVolume(message.value);
          }
          break;
        case 'setContinueMode':
          if (typeof message.value === 'string') {
            await this.client.send('setContinueMode', { mode: message.value });
          }
          break;
      }
    } catch (err) {
      vscode.window.showErrorMessage(`Command failed: ${err}`);
    }
  }

  private startStatusUpdates(): void {
    // Poll status at 2Hz (UI updates)
    this.statusInterval = setInterval(async () => {
      try {
        if (this.client.isConnected()) {
          const status = await this.client.getStatus();
          this.panel.webview.postMessage({ type: 'status', data: status });
        }
      } catch {
        // Ignore errors during status polling
      }
    }, 500);

    // Use push-based streaming for real-time audio data (~60fps from daemon)
    let audioFrameCount = 0;
    const audioStartTime = Date.now();
    
    // Handler for streamed audio data
    const audioHandler = (audioData: { bands: number[] }) => {
      audioFrameCount++;
      this.panel.webview.postMessage({ type: 'audioData', data: audioData });
      
      // Log FPS every 5 seconds (~300 frames at 60fps)
      if (audioFrameCount % 300 === 0) {
        const elapsed = (Date.now() - audioStartTime) / 1000;
        console.log(`[PlayerView] Audio streaming: ${(audioFrameCount / elapsed).toFixed(1)} fps`);
      }
    };
    
    // Subscribe to audio data stream
    this.client.on('audioData', audioHandler);
    this.client.subscribeAudioData().catch(err => {
      console.error('[PlayerView] Failed to subscribe to audio data:', err);
    });
    
    // Cleanup on dispose
    this.disposables.push({
      dispose: () => {
        this.client.removeListener('audioData', audioHandler);
        this.client.unsubscribeAudioData().catch(() => {});
      }
    });
  }

  private update(): void {
    this.panel.title = 'Music Player';
    this.panel.webview.html = this.getHtmlContent();
  }

  private getHtmlContent(): string {
    return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Music Player</title>
    <style>
        :root {
            --bg-primary: #0d1117;
            --bg-secondary: #1a1a2e;
            --bg-tertiary: #16213e;
            --bar-bg: linear-gradient(180deg, #2d2d44 0%, #1a1a2e 50%, #0f0f1a 100%);
            --bar-border: #3d3d5c;
            --text-primary: #ffffff;
            --text-secondary: #a0a0b0;
            --text-muted: #6a6a7a;
            --accent: #ff6b35;
            --accent-hover: #ff8c5a;
            --progress-bg: #3d3d5c;
            --progress-fill: linear-gradient(90deg, #ff6b35, #ff8c5a);
            --progress-buffered: #4a4a6a;
        }

        * { margin: 0; padding: 0; box-sizing: border-box; }

        html, body {
            font-family: 'Segoe UI', 'Trebuchet MS', system-ui, -apple-system, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            height: 100%;
            overflow: hidden;
        }

        /* Visualizer canvas - full background */
        #visualizer {
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: calc(100% - 72px);
            z-index: 0;
        }

        /* MySpace-style bottom player bar */
        .player-bar {
            position: fixed;
            bottom: 0;
            left: 0;
            right: 0;
            height: 72px;
            background: var(--bar-bg);
            border-top: 1px solid var(--bar-border);
            display: flex;
            align-items: center;
            padding: 0 16px;
            z-index: 100;
            box-shadow: 0 -4px 20px rgba(0, 0, 0, 0.5);
        }

        /* Left section - Controls */
        .controls {
            display: flex;
            align-items: center;
            gap: 4px;
            flex-shrink: 0;
        }

        .control-btn {
            background: linear-gradient(180deg, #4a4a6a 0%, #2d2d44 100%);
            border: 1px solid #5a5a7a;
            color: var(--text-primary);
            cursor: pointer;
            width: 36px;
            height: 36px;
            border-radius: 4px;
            display: flex;
            align-items: center;
            justify-content: center;
            transition: all 0.15s ease;
            box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3), inset 0 1px 0 rgba(255, 255, 255, 0.1);
        }

        .control-btn:hover {
            background: linear-gradient(180deg, #5a5a7a 0%, #3d3d54 100%);
            border-color: var(--accent);
        }

        .control-btn:active {
            background: linear-gradient(180deg, #2d2d44 0%, #4a4a6a 100%);
            box-shadow: inset 0 2px 4px rgba(0, 0, 0, 0.3);
        }

        .control-btn.primary {
            width: 44px;
            height: 44px;
            background: linear-gradient(180deg, var(--accent) 0%, #cc5429 100%);
            border-color: #ff8c5a;
        }

        .control-btn.primary:hover {
            background: linear-gradient(180deg, var(--accent-hover) 0%, var(--accent) 100%);
        }

        .control-btn svg { width: 18px; height: 18px; fill: currentColor; }
        .control-btn.primary svg { width: 22px; height: 22px; }
        
        .control-btn.active {
            background: linear-gradient(180deg, var(--accent) 0%, #cc5429 100%);
            border-color: #ff8c5a;
            color: white;
        }
        
        .control-btn.active:hover {
            background: linear-gradient(180deg, var(--accent-hover) 0%, var(--accent) 100%);
        }

        /* Center section - Progress */
        .progress-section {
            flex: 1;
            display: flex;
            align-items: center;
            gap: 12px;
            margin: 0 20px;
            min-width: 0;
        }

        .time-current, .time-total {
            font-size: 11px;
            color: var(--text-muted);
            font-family: 'Consolas', 'Monaco', monospace;
            min-width: 40px;
            text-align: center;
        }

        .progress-bar-container {
            flex: 1;
            height: 20px;
            display: flex;
            align-items: center;
            cursor: pointer;
        }

        .progress-bar {
            width: 100%;
            height: 8px;
            background: var(--progress-bg);
            border-radius: 4px;
            position: relative;
            overflow: hidden;
            box-shadow: inset 0 1px 3px rgba(0, 0, 0, 0.4);
        }

        .progress-fill {
            height: 100%;
            background: var(--progress-fill);
            border-radius: 4px;
            position: relative;
            transition: width 0.1s linear;
        }

        .progress-fill::after {
            content: '';
            position: absolute;
            right: 0;
            top: 50%;
            transform: translateY(-50%);
            width: 12px;
            height: 12px;
            background: #fff;
            border-radius: 50%;
            box-shadow: 0 0 4px rgba(0, 0, 0, 0.5);
        }

        /* Right section - Track info & Volume */
        .info-section {
            display: flex;
            align-items: center;
            gap: 16px;
            flex-shrink: 0;
            max-width: 350px;
        }

        .track-info {
            display: flex;
            flex-direction: column;
            min-width: 0;
            max-width: 200px;
        }

        .track-title {
            font-size: 13px;
            font-weight: 600;
            color: var(--text-primary);
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        .track-artist {
            font-size: 11px;
            color: var(--text-secondary);
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        .track-album {
            font-size: 10px;
            color: var(--text-muted);
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
            font-style: italic;
        }

        /* Volume control */
        .volume-container {
            display: flex;
            align-items: center;
            gap: 6px;
        }

        .volume-icon {
            color: var(--text-secondary);
            cursor: pointer;
        }

        .volume-icon:hover { color: var(--accent); }
        .volume-icon svg { width: 18px; height: 18px; }

        .volume-slider {
            width: 80px;
            -webkit-appearance: none;
            height: 6px;
            background: var(--progress-bg);
            border-radius: 3px;
            cursor: pointer;
        }

        .volume-slider::-webkit-slider-thumb {
            -webkit-appearance: none;
            width: 12px;
            height: 12px;
            background: var(--accent);
            border-radius: 50%;
            cursor: pointer;
            box-shadow: 0 0 4px rgba(0, 0, 0, 0.4);
        }

        .volume-slider::-webkit-slider-thumb:hover {
            background: var(--accent-hover);
        }

        /* Not playing state */
        .not-playing-bar {
            flex: 1;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 12px;
            color: var(--text-muted);
        }

        .not-playing-bar svg {
            width: 24px;
            height: 24px;
            opacity: 0.5;
        }

        .not-playing-bar span {
            font-size: 13px;
        }

        /* Center info display area (above player bar) */
        .center-display {
            position: fixed;
            bottom: 92px;
            left: 50%;
            transform: translateX(-50%);
            background: rgba(26, 26, 46, 0.9);
            backdrop-filter: blur(10px);
            border: 1px solid var(--bar-border);
            border-radius: 12px;
            padding: 20px 32px;
            text-align: center;
            z-index: 50;
            min-width: 300px;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
        }

        .center-display.hidden { display: none; }

        .center-track-title {
            font-size: 1.5rem;
            font-weight: 700;
            color: var(--text-primary);
            margin-bottom: 8px;
        }

        .center-track-artist {
            font-size: 1.1rem;
            color: var(--accent);
            margin-bottom: 4px;
        }

        .center-track-album {
            font-size: 0.9rem;
            color: var(--text-secondary);
            font-style: italic;
        }

        /* Queue position indicator */
        .queue-info {
            font-size: 10px;
            color: var(--text-muted);
            margin-left: 12px;
            padding: 2px 6px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 3px;
        }
    </style>
</head>
<body>
    <canvas id="visualizer"></canvas>
    
    <!-- Center display for track info -->
    <div id="centerDisplay" class="center-display hidden">
        <div class="center-track-title" id="centerTitle">-</div>
        <div class="center-track-artist" id="centerArtist">-</div>
        <div class="center-track-album" id="centerAlbum"></div>
    </div>

    <!-- Bottom player bar -->
    <div class="player-bar" id="playerBar">
        <div id="content" style="display: contents;">
            <!-- Controls left -->
            <div class="controls">
                <button class="control-btn" id="prevBtn" title="Previous">
                    <svg viewBox="0 0 24 24"><path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/></svg>
                </button>
                <button class="control-btn primary" id="playPauseBtn" title="Play">
                    <svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg>
                </button>
                <button class="control-btn" id="nextBtn" title="Next">
                    <svg viewBox="0 0 24 24"><path d="M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z"/></svg>
                </button>
                <button class="control-btn" id="continueModeBtn" title="Continue with similar music when queue ends">
                    <svg viewBox="0 0 24 24"><path d="M18.6 6.62c-1.44 0-2.8.56-3.77 1.53L12 10.66 10.48 12h.01L7.8 14.39c-.64.64-1.49.99-2.4.99-1.87 0-3.39-1.51-3.39-3.38S3.53 8.62 5.4 8.62c.91 0 1.76.35 2.44 1.03l1.13 1 1.51-1.34L9.22 8.2C8.2 7.18 6.84 6.62 5.4 6.62 2.42 6.62 0 9.04 0 12s2.42 5.38 5.4 5.38c1.44 0 2.8-.56 3.77-1.53l2.83-2.5.01.01L13.52 12h-.01l2.69-2.39c.64-.64 1.49-.99 2.4-.99 1.87 0 3.39 1.51 3.39 3.38s-1.52 3.38-3.39 3.38c-.9 0-1.76-.35-2.44-1.03l-1.14-1.01-1.51 1.34 1.27 1.12c1.02 1.01 2.37 1.57 3.82 1.57 2.98 0 5.4-2.41 5.4-5.38s-2.42-5.37-5.4-5.37z"/></svg>
                </button>
            </div>

            <!-- Progress center -->
            <div class="progress-section">
                <span class="time-current" id="timeCurrent">0:00</span>
                <div class="progress-bar-container" id="progressBar">
                    <div class="progress-bar">
                        <div class="progress-fill" id="progressFill" style="width: 0%"></div>
                    </div>
                </div>
                <span class="time-total" id="timeTotal">0:00</span>
            </div>

            <!-- Info & Volume right -->
            <div class="info-section">
                <div class="track-info">
                    <div class="track-title" id="trackTitle">No track playing</div>
                    <div class="track-artist" id="trackArtist">Select a track from your library</div>
                    <div class="track-album" id="trackAlbum"></div>
                </div>
                <span class="queue-info" id="queueInfo" style="display: none;">1/1</span>
                <div class="volume-container">
                    <div class="volume-icon" id="volumeIcon" title="Mute">
                        <svg viewBox="0 0 24 24" fill="currentColor">
                            <path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02z"/>
                        </svg>
                    </div>
                    <input type="range" class="volume-slider" id="volumeSlider" min="0" max="100" value="100">
                </div>
            </div>
        </div>
    </div>

    <script>
        const vscode = acquireVsCodeApi();
        let currentStatus = null;
        
        // Use typed array for better performance (128 bands to match original)
        let audioBands = new Uint8Array(128);
        let audioLogCounter = 0;
        let debugOffsetSum = 0;      // For calculating average offset
        let debugOffsetCount = 0;    // Number of offset samples
        
        // Audio delay correction buffer
        // Buffers incoming audio data and applies it when playback catches up
        const audioBuffer = [];
        const MAX_BUFFER_SIZE = 100;  // ~2 seconds of audio frames
        let lastStatusPosition = 0;   // Last known playback position from status
        let lastStatusTime = 0;       // When we received that status
        let isPlaying = false;        // Track playback state for interpolation
        
        // Additional latency compensation (ms) for audio driver/speaker delay
        // Positive = delay visualization more (if visualization is ahead of audio)
        // Negative = speed up visualization (if visualization is behind audio)
        // Typical values: 50-150ms depending on audio hardware
        const AUDIO_LATENCY_OFFSET_MS = 255;
        
        // Interpolate current playback position between status updates
        // Status updates only come every 500ms, so we estimate position in between
        function getInterpolatedPosition() {
            if (!isPlaying || lastStatusTime === 0) {
                return lastStatusPosition;
            }
            const elapsed = Date.now() - lastStatusTime;
            // Subtract latency offset to delay visualization further
            return lastStatusPosition + elapsed - AUDIO_LATENCY_OFFSET_MS;
        }
        
        console.log('[Visualizer] Player webview loaded and running');

        // ========== Modern Particle Visualizer ==========
        // Config matching original CodePen visualizer
        const CONFIG = {
            particleCount: 150,
            numBands: 128,  // Match original
            colors: ['#69D2E7', '#1B676B', '#BEF202', '#EBE54D', '#00CDAC', '#1693A5', '#F9D423', '#FF4E50', '#E7204E', '#0CCABA', '#FF006F'],
            scale: { min: 5.0, max: 80.0 },
            speed: { min: 0.2, max: 1.0 },
            alpha: { min: 0.8, max: 0.9 },  // Match original (was 0.7)
            spin: { min: 0.001, max: 0.005 },
            size: { min: 0.5, max: 1.25 },
            noiseOpacity: 0.005
        };
        
        const canvas = document.getElementById('visualizer');
        const ctx = canvas.getContext('2d', { alpha: false });
        
        // Device pixel ratio for crisp rendering
        let dpr = Math.min(window.devicePixelRatio || 1, 2);
        let width = 0;
        let height = 0;
        
        // Procedural noise pattern (generated once)
        let noisePattern = null;
        
        function createNoisePattern() {
            const size = 128;
            const noiseCanvas = document.createElement('canvas');
            noiseCanvas.width = size;
            noiseCanvas.height = size;
            const noiseCtx = noiseCanvas.getContext('2d');
            const imageData = noiseCtx.createImageData(size, size);
            const data = imageData.data;
            
            for (let i = 0; i < data.length; i += 4) {
                const value = Math.random() * 255;
                data[i] = value;
                data[i + 1] = value;
                data[i + 2] = value;
                data[i + 3] = 255;
            }
            
            noiseCtx.putImageData(imageData, 0, 0);
            return ctx.createPattern(noiseCanvas, 'repeat');
        }

        function resizeCanvas() {
            dpr = Math.min(window.devicePixelRatio || 1, 2);
            width = window.innerWidth;
            height = window.innerHeight;
            
            canvas.width = width * dpr;
            canvas.height = height * dpr;
            canvas.style.width = width + 'px';
            canvas.style.height = height + 'px';
            
            ctx.scale(dpr, dpr);
            
            // Recreate noise pattern at new scale
            noisePattern = createNoisePattern();
            
            // Reinitialize particles for new dimensions
            initParticles();
        }

        // Use ResizeObserver for efficient resize handling
        if (typeof ResizeObserver !== 'undefined') {
            new ResizeObserver(() => resizeCanvas()).observe(document.body);
        } else {
            window.addEventListener('resize', resizeCanvas);
        }

        // Utility functions
        const random = (min, max) => max === undefined 
            ? Math.random() * min 
            : min + Math.random() * (max - min);
        
        const randomInt = (min, max) => Math.floor(random(min, max));
        const pick = (arr) => arr[randomInt(0, arr.length)];

        // Particle class - matching original CodePen behavior
        class Particle {
            x = 0; y = 0; level = 1; scale = 0; alpha = 0;
            speed = 0; color = ''; size = 0; spin = 0; band = 0;
            smoothedScale = 0; smoothedAlpha = 0;
            decayScale = 0; decayAlpha = 0;
            rotation = 0; energy = 0;

            constructor() {
                this.reset(true);
            }

            reset(initial = false) {
                this.x = random(0, width);
                this.y = initial ? random(0, height * 2) : height + random(0, 100);
                this.level = 1 + randomInt(0, 4);
                this.scale = random(CONFIG.scale.min, CONFIG.scale.max);
                this.alpha = random(CONFIG.alpha.min, CONFIG.alpha.max);
                this.speed = random(CONFIG.speed.min, CONFIG.speed.max);
                this.color = pick(CONFIG.colors);
                this.size = random(CONFIG.size.min, CONFIG.size.max);
                this.spin = random(CONFIG.spin.min, CONFIG.spin.max) * (Math.random() < 0.5 ? 1 : -1);
                this.band = randomInt(0, CONFIG.numBands);
                this.rotation = random(0, Math.PI * 2);
                
                // Reset smoothing state (matching original)
                this.smoothedScale = 0;
                this.smoothedAlpha = 0;
                this.decayScale = 0;
                this.decayAlpha = 0;
                this.energy = 0;
            }

            update(bands, hasNewData) {
                // Only update energy when new audio data arrives
                // This allows decay to happen between audio updates
                if (hasNewData) {
                    const bandValue = bands[this.band] || 0;
                    this.energy = bandValue / 255;
                }
                
                // Movement (happens every frame)
                this.rotation += this.spin;
                this.y -= this.speed * this.level;

                // Reset when off screen
                const maxSize = this.size * this.level * this.scale * 2;
                if (this.y < -maxSize) {
                    this.reset();
                    this.y = height + this.size * this.scale * this.level;
                }
            }

            draw(ctx) {
                // Original player.html logic - exact copy
                const power = Math.exp(this.energy);
                const scale = this.scale * power;
                const alpha = this.alpha * this.energy * 1.5;
                
                this.decayScale = Math.max(this.decayScale, scale);
                this.decayAlpha = Math.max(this.decayAlpha, alpha);
                this.smoothedScale += (this.decayScale - this.smoothedScale) * 0.3;
                this.smoothedAlpha += (this.decayAlpha - this.smoothedAlpha) * 0.3;
                this.decayScale *= 0.985;
                this.decayAlpha *= 0.975;
                
                ctx.save();
                ctx.beginPath();
                ctx.translate(this.x + Math.cos(this.rotation * this.speed) * 250, this.y);
                ctx.rotate(this.rotation);
                ctx.scale(this.smoothedScale * this.level, this.smoothedScale * this.level);
                ctx.moveTo(this.size * 0.5, 0);
                ctx.lineTo(this.size * -0.5, 0);
                ctx.lineWidth = 1;
                ctx.lineCap = 'round';
                ctx.globalAlpha = this.smoothedAlpha / this.level;
                ctx.strokeStyle = this.color;
                ctx.stroke();
                ctx.restore();
            }
        }

        // Particle pool
        let particles = [];

        function initParticles() {
            particles = [];
            for (let i = 0; i < CONFIG.particleCount; i++) {
                particles.push(new Particle());
            }
        }

        // Animation loop with timing
        let lastTime = 0;
        
        function animate(timestamp) {
            lastTime = timestamp;

            // Clear the canvas completely each frame (no trail)
            ctx.fillStyle = '#0d1117';
            ctx.fillRect(0, 0, width, height);

            // Draw particles with additive blending for glow effect
            ctx.globalCompositeOperation = 'lighter';
            
            // Apply buffered audio data based on current playback position
            // This syncs visualization with what the user actually hears
            const currentPos = getInterpolatedPosition();
            let hasNew = false;
            
            // Apply all buffered frames up to the current playback position
            while (audioBuffer.length > 0 && audioBuffer[0].position <= currentPos) {
                const frame = audioBuffer.shift();
                const bands = frame.bands;
                const len = Math.min(128, bands.length);
                for (let i = 0; i < len; i++) {
                    audioBands[i] = bands[i];
                }
                hasNew = true;
            }
            
            for (const particle of particles) {
                particle.update(audioBands, hasNew);
                particle.draw(ctx);
            }
            
            ctx.globalCompositeOperation = 'source-over';

            // Very subtle noise overlay for texture (optional)
            if (noisePattern && CONFIG.noiseOpacity > 0) {
                ctx.globalAlpha = CONFIG.noiseOpacity;
                ctx.fillStyle = noisePattern;
                ctx.fillRect(0, 0, width, height);
                ctx.globalAlpha = 1;
            }

            requestAnimationFrame(animate);
        }

        // Initialize
        resizeCanvas();
        requestAnimationFrame(animate);

        // ========== Player Logic ==========
        function formatTime(ms) {
            const totalSeconds = Math.floor(ms / 1000);
            const minutes = Math.floor(totalSeconds / 60);
            const seconds = totalSeconds % 60;
            return minutes + ':' + seconds.toString().padStart(2, '0');
        }

        // Volume icons for different states
        const volumeIcons = {
            muted: '<path d="M16.5 12c0-1.77-1.02-3.29-2.5-4.03v2.21l2.45 2.45c.03-.2.05-.41.05-.63zm2.5 0c0 .94-.2 1.82-.54 2.64l1.51 1.51C20.63 14.91 21 13.5 21 12c0-4.28-2.99-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71zM4.27 3L3 4.27 7.73 9H3v6h4l5 5v-6.73l4.25 4.25c-.67.52-1.42.93-2.25 1.18v2.06c1.38-.31 2.63-.95 3.69-1.81L19.73 21 21 19.73l-9-9L4.27 3zM12 4L9.91 6.09 12 8.18V4z"/>',
            low: '<path d="M7 9v6h4l5 5V4l-5 5H7z"/>',
            medium: '<path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02z"/>',
            high: '<path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z"/>'
        };

        function getVolumeIcon(volume) {
            if (volume === 0) return volumeIcons.muted;
            if (volume < 0.33) return volumeIcons.low;
            if (volume < 0.66) return volumeIcons.medium;
            return volumeIcons.high;
        }

        let lastVolume = 1; // For mute toggle
        let hasTrack = false;

        function updatePlayer(status) {
            const isPlaying = status.state === 'playing';
            hasTrack = status.path && status.state !== 'stopped';
            
            // Update center display
            const centerDisplay = document.getElementById('centerDisplay');
            const centerTitle = document.getElementById('centerTitle');
            const centerArtist = document.getElementById('centerArtist');
            const centerAlbum = document.getElementById('centerAlbum');
            
            if (hasTrack) {
                centerDisplay.classList.remove('hidden');
                const title = status.metadata?.title || status.path.split('/').pop() || 'Unknown';
                const artist = status.metadata?.artist || 'Unknown Artist';
                const album = status.metadata?.album || '';
                
                centerTitle.textContent = title;
                centerArtist.textContent = artist;
                centerAlbum.textContent = album;
            } else {
                centerDisplay.classList.add('hidden');
            }
            
            // Update bottom bar elements
            const playPauseBtn = document.getElementById('playPauseBtn');
            const progressFill = document.getElementById('progressFill');
            const timeCurrent = document.getElementById('timeCurrent');
            const timeTotal = document.getElementById('timeTotal');
            const trackTitle = document.getElementById('trackTitle');
            const trackArtist = document.getElementById('trackArtist');
            const trackAlbum = document.getElementById('trackAlbum');
            const volumeSlider = document.getElementById('volumeSlider');
            const volumeIcon = document.getElementById('volumeIcon');
            const queueInfo = document.getElementById('queueInfo');
            
            // Play/Pause button icon
            if (isPlaying) {
                playPauseBtn.innerHTML = '<svg viewBox="0 0 24 24"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/></svg>';
                playPauseBtn.title = 'Pause';
            } else {
                playPauseBtn.innerHTML = '<svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg>';
                playPauseBtn.title = 'Play';
            }
            
            // Progress
            const progress = status.duration > 0 ? (status.position / status.duration * 100) : 0;
            progressFill.style.width = progress + '%';
            timeCurrent.textContent = formatTime(status.position || 0);
            timeTotal.textContent = formatTime(status.duration || 0);
            
            // Track info in bottom bar
            if (hasTrack) {
                const title = status.metadata?.title || status.path.split('/').pop() || 'Unknown';
                const artist = status.metadata?.artist || 'Unknown Artist';
                const album = status.metadata?.album || '';
                
                trackTitle.textContent = title;
                trackArtist.textContent = artist;
                trackAlbum.textContent = album;
            } else {
                trackTitle.textContent = 'No track playing';
                trackArtist.textContent = 'Select a track from your library';
                trackAlbum.textContent = '';
                
                // Reset audio bands when stopped
                audioBands.fill(0);
            }
            
            // Queue info
            if (status.queueSize > 0) {
                queueInfo.style.display = 'inline';
                queueInfo.textContent = (status.queueIndex + 1) + '/' + status.queueSize;
            } else {
                queueInfo.style.display = 'none';
            }
            
            // Volume (don't update while dragging)
            if (!volumeSlider.matches(':active')) {
                volumeSlider.value = Math.round((status.volume || 0) * 100);
            }
            volumeIcon.innerHTML = '<svg viewBox="0 0 24 24" fill="currentColor">' + getVolumeIcon(status.volume || 0) + '</svg>';
        }
        
        // Set up event listeners once (not on every render)
        function initEventListeners() {
            document.getElementById('prevBtn').addEventListener('click', () => {
                vscode.postMessage({ command: 'previous' });
            });

            document.getElementById('playPauseBtn').addEventListener('click', () => {
                vscode.postMessage({ command: currentStatus?.state === 'playing' ? 'pause' : 'play' });
            });

            document.getElementById('nextBtn').addEventListener('click', () => {
                vscode.postMessage({ command: 'next' });
            });
            
            // Continue mode toggle
            let continueMode = 'off';
            const continueModeBtn = document.getElementById('continueModeBtn');
            continueModeBtn.addEventListener('click', () => {
                // Cycle through modes: off -> similar -> off
                continueMode = continueMode === 'off' ? 'similar' : 'off';
                vscode.postMessage({ command: 'setContinueMode', value: continueMode });
                updateContinueModeButton();
            });
            
            function updateContinueModeButton() {
                if (continueMode === 'similar') {
                    continueModeBtn.classList.add('active');
                    continueModeBtn.title = 'Continue with similar music: ON';
                } else {
                    continueModeBtn.classList.remove('active');
                    continueModeBtn.title = 'Continue with similar music when queue ends';
                }
            }

            // Progress bar click to seek
            const progressContainer = document.getElementById('progressBar');
            progressContainer.addEventListener('click', (e) => {
                if (!currentStatus?.duration) return;
                const rect = progressContainer.getBoundingClientRect();
                const percent = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
                const position = Math.round(percent * currentStatus.duration);
                vscode.postMessage({ command: 'seek', value: position });
            });

            // Volume slider
            document.getElementById('volumeSlider').addEventListener('input', (e) => {
                const volume = parseInt(e.target.value) / 100;
                lastVolume = volume > 0 ? volume : lastVolume;
                vscode.postMessage({ command: 'volume', value: volume });
            });
            
            // Volume icon click to mute/unmute
            document.getElementById('volumeIcon').addEventListener('click', () => {
                const current = currentStatus?.volume || 0;
                if (current > 0) {
                    lastVolume = current;
                    vscode.postMessage({ command: 'volume', value: 0 });
                } else {
                    vscode.postMessage({ command: 'volume', value: lastVolume || 1 });
                }
            });
        }
        
        // Initialize event listeners
        initEventListeners();

        // Handle messages from the extension
        let messageCount = 0;
        window.addEventListener('message', event => {
            const message = event.data;
            messageCount++;
            if (messageCount <= 3) {
                console.log('[Visualizer] Message received:', message.type, message.data ? 'with data' : 'no data');
            }
            if (message.type === 'status') {
                currentStatus = message.data;
                
                // Track position for audio delay correction interpolation
                lastStatusPosition = message.data.position || 0;
                lastStatusTime = Date.now();
                isPlaying = message.data.state === 'playing';
                
                // Log debug info from extension
                if (message.debug) {
                    console.log('[Visualizer] Audio polling:', message.debug);
                }
                updatePlayer(currentStatus);
            } else if (message.type === 'audioData') {
                // Buffer audio data for delay correction
                // Data is applied in the animation loop when playback catches up
                if (message.data?.bands) {
                    audioBuffer.push({
                        bands: message.data.bands,
                        position: message.data.position || 0
                    });
                    
                    // Trim old entries to prevent memory growth
                    while (audioBuffer.length > MAX_BUFFER_SIZE) {
                        audioBuffer.shift();
                    }
                    
                    // Track offset for debugging (how far ahead audio data is)
                    const audioDataPos = message.data.position || 0;
                    const currentPos = getInterpolatedPosition();
                    const offset = audioDataPos - currentPos;
                    debugOffsetSum += offset;
                    debugOffsetCount++;
                    
                    // Debug logging (throttled to ~1 per second)
                    audioLogCounter++;
                    if (audioLogCounter % 60 === 0) {
                        const bands = message.data.bands;
                        const sum = bands.reduce((a, b) => a + b, 0);
                        const nonZero = bands.filter(b => b > 0).length;
                        const max = Math.max(...bands);
                        const avgOffset = debugOffsetCount > 0 ? Math.round(debugOffsetSum / debugOffsetCount) : 0;
                        console.log('[Visualizer] Audio data:', {
                            fps: audioLogCounter + ' frames in ~1s',
                            bands: bands.length,
                            nonZero: nonZero + '/' + bands.length,
                            sum,
                            max,
                            bufferSize: audioBuffer.length,
                            avgOffsetMs: avgOffset,
                            sample: [bands[0], bands[16], bands[32], bands[64], bands[96], bands[127]]
                        });
                        audioLogCounter = 0;
                        debugOffsetSum = 0;
                        debugOffsetCount = 0;
                    }
                }
            }
        });

        // Initial render
        updatePlayer({ state: 'stopped', position: 0, duration: 0, volume: 1, queueIndex: 0, queueSize: 0 });
    </script>
</body>
</html>`;
  }

  public dispose(): void {
    PlayerWebviewPanel.currentPanel = undefined;

    if (this.statusInterval) {
      clearInterval(this.statusInterval);
    }

    // Cleanup is handled by disposables for polling interval

    this.panel.dispose();

    while (this.disposables.length) {
      const x = this.disposables.pop();
      if (x) {
        x.dispose();
      }
    }
  }
}
