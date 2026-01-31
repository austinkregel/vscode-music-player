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
            --bg-secondary: #161b22;
            --bg-tertiary: #21262d;
            --text-primary: #f0f6fc;
            --text-secondary: #8b949e;
            --accent: #58a6ff;
            --accent-hover: #79c0ff;
            --progress-bg: #30363d;
            --progress-fill: #58a6ff;
        }

        * { margin: 0; padding: 0; box-sizing: border-box; }

        html, body {
            font-family: 'Segoe UI', system-ui, -apple-system, sans-serif;
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
            height: 100%;
            z-index: 0;
        }

        /* Player UI overlay */
        .player-container {
            position: relative;
            z-index: 1;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }

        .player {
            background: rgba(22, 27, 34, 0.85);
            backdrop-filter: blur(20px);
            border-radius: 16px;
            padding: 32px;
            max-width: 400px;
            width: 100%;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
        }

        .visualizer-box {
            width: 100%;
            aspect-ratio: 1;
            background: rgba(33, 38, 45, 0.5);
            border-radius: 12px;
            margin-bottom: 24px;
            position: relative;
            overflow: hidden;
        }

        .visualizer-box canvas {
            position: absolute;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
        }

        .track-info {
            text-align: center;
            margin-bottom: 24px;
        }

        .track-title {
            font-size: 1.4rem;
            font-weight: 600;
            margin-bottom: 8px;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }

        .track-artist {
            color: var(--text-secondary);
            font-size: 1rem;
        }

        .progress-container { margin-bottom: 20px; }

        .progress-bar {
            width: 100%;
            height: 6px;
            background: var(--progress-bg);
            border-radius: 3px;
            cursor: pointer;
            position: relative;
        }

        .progress-fill {
            height: 100%;
            background: var(--progress-fill);
            border-radius: 3px;
            transition: width 0.1s ease;
        }

        .time-display {
            display: flex;
            justify-content: space-between;
            font-size: 0.85rem;
            color: var(--text-secondary);
            margin-top: 8px;
        }

        .controls {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 24px;
        }

        .control-btn {
            background: none;
            border: none;
            color: var(--text-primary);
            cursor: pointer;
            padding: 12px;
            border-radius: 50%;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            justify-content: center;
        }

        .control-btn:hover {
            background: var(--bg-tertiary);
            color: var(--accent);
        }

        .control-btn.primary {
            background: var(--accent);
            color: var(--bg-primary);
            width: 64px;
            height: 64px;
        }

        .control-btn.primary:hover { background: var(--accent-hover); }
        .control-btn svg { width: 24px; height: 24px; }
        .control-btn.primary svg { width: 32px; height: 32px; }

        .volume-container {
            display: flex;
            align-items: center;
            gap: 12px;
            margin-top: 24px;
        }

        .volume-slider {
            flex: 1;
            -webkit-appearance: none;
            height: 4px;
            background: var(--progress-bg);
            border-radius: 2px;
            cursor: pointer;
        }

        .volume-slider::-webkit-slider-thumb {
            -webkit-appearance: none;
            width: 14px;
            height: 14px;
            background: var(--accent);
            border-radius: 50%;
            cursor: pointer;
        }

        .not-playing {
            text-align: center;
            color: var(--text-secondary);
            padding: 40px 20px;
        }

        .not-playing svg {
            width: 64px;
            height: 64px;
            margin-bottom: 16px;
            opacity: 0.5;
        }
    </style>
</head>
<body>
    <canvas id="visualizer"></canvas>
    <div class="player-container">
        <div class="player">
            <div id="content">
                <div class="not-playing">
                    <svg viewBox="0 0 24 24" fill="currentColor">
                        <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z"/>
                    </svg>
                    <p>No track playing</p>
                    <p style="font-size: 0.9em; margin-top: 8px;">Select a track from your library to start playing</p>
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

        function renderPlayer(status) {
            const content = document.getElementById('content');
            
            if (status.state === 'stopped' || !status.path) {
                content.innerHTML = \`
                    <div class="not-playing">
                        <svg viewBox="0 0 24 24" fill="currentColor">
                            <path d="M12 3v10.55c-.59-.34-1.27-.55-2-.55-2.21 0-4 1.79-4 4s1.79 4 4 4 4-1.79 4-4V7h4V3h-6z"/>
                        </svg>
                        <p>No track playing</p>
                        <p style="font-size: 0.9em; margin-top: 8px;">Select a track from your library to start playing</p>
                    </div>
                \`;
                // Reset audio bands when stopped
                audioBands.fill(0);
                return;
            }

            const title = status.metadata?.title || status.path.split('/').pop() || 'Unknown';
            const artist = status.metadata?.artist || 'Unknown Artist';
            const progress = status.duration > 0 ? (status.position / status.duration * 100) : 0;
            const playIcon = status.state === 'playing' 
                ? '<path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>'
                : '<path d="M8 5v14l11-7z"/>';

            content.innerHTML = \`
                <div class="track-info">
                    <div class="track-title">\${title}</div>
                    <div class="track-artist">\${artist}</div>
                </div>

                <div class="progress-container">
                    <div class="progress-bar" id="progressBar">
                        <div class="progress-fill" style="width: \${progress}%"></div>
                    </div>
                    <div class="time-display">
                        <span>\${formatTime(status.position)}</span>
                        <span>\${formatTime(status.duration)}</span>
                    </div>
                </div>

                <div class="controls">
                    <button class="control-btn" id="prevBtn" title="Previous">
                        <svg viewBox="0 0 24 24" fill="currentColor">
                            <path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/>
                        </svg>
                    </button>
                    <button class="control-btn primary" id="playPauseBtn" title="Play/Pause">
                        <svg viewBox="0 0 24 24" fill="currentColor">
                            \${playIcon}
                        </svg>
                    </button>
                    <button class="control-btn" id="nextBtn" title="Next">
                        <svg viewBox="0 0 24 24" fill="currentColor">
                            <path d="M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z"/>
                        </svg>
                    </button>
                </div>

                <div class="volume-container">
                    <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
                        <path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02z"/>
                    </svg>
                    <input type="range" class="volume-slider" id="volumeSlider" min="0" max="100" value="\${Math.round(status.volume * 100)}">
                </div>
            \`;

            // Add event listeners
            document.getElementById('prevBtn').addEventListener('click', () => {
                vscode.postMessage({ command: 'previous' });
            });

            document.getElementById('playPauseBtn').addEventListener('click', () => {
                vscode.postMessage({ command: status.state === 'playing' ? 'pause' : 'play' });
            });

            document.getElementById('nextBtn').addEventListener('click', () => {
                vscode.postMessage({ command: 'next' });
            });

            document.getElementById('progressBar').addEventListener('click', (e) => {
                const rect = e.target.getBoundingClientRect();
                const percent = (e.clientX - rect.left) / rect.width;
                const position = Math.round(percent * status.duration);
                vscode.postMessage({ command: 'seek', value: position });
            });

            document.getElementById('volumeSlider').addEventListener('input', (e) => {
                const volume = parseInt(e.target.value) / 100;
                vscode.postMessage({ command: 'volume', value: volume });
            });
        }

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
                renderPlayer(currentStatus);
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
        renderPlayer({ state: 'stopped', position: 0, duration: 0, volume: 1 });
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
