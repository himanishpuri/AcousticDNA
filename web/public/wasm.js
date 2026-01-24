/**
 * AcousticDNA WASM Loader
 * 
 * This module handles loading and interfacing with the AcousticDNA WebAssembly module.
 * It provides a clean JavaScript API for audio fingerprinting in the browser.
 */

class WASMLoader {
    constructor() {
        this.ready = false;
        this.loading = false;
        this.generateFingerprint = null;
        this.wasmInstance = null;
        this.audioContext = null;
    }

    /**
     * Initialize the WASM module
     * @returns {Promise<void>}
     */
    async init() {
        if (this.ready) {
            console.log('✅ WASM already initialized');
            return;
        }

        if (this.loading) {
            console.log('⏳ WASM initialization already in progress');
            // Wait for the existing initialization to complete
            return new Promise((resolve) => {
                const checkReady = setInterval(() => {
                    if (this.ready) {
                        clearInterval(checkReady);
                        resolve();
                    }
                }, 100);
            });
        }

        this.loading = true;
        console.log('🔧 Initializing WASM module...');

        try {
            // Load wasm_exec.js runtime
            await this._loadWasmExec();

            // Initialize Go WASM runtime
            const go = new window.Go();

            // Load and instantiate WASM module
            const wasmPath = 'fingerprint.wasm';
            console.log(`📥 Loading WASM from ${wasmPath}...`);

            const startTime = performance.now();

            let wasmInstance;
            if (WebAssembly.instantiateStreaming) {
                // Modern browsers support streaming
                const response = await fetch(wasmPath);
                const result = await WebAssembly.instantiateStreaming(response, go.importObject);
                wasmInstance = result.instance;
            } else {
                // Fallback for older browsers
                const response = await fetch(wasmPath);
                const buffer = await response.arrayBuffer();
                const result = await WebAssembly.instantiate(buffer, go.importObject);
                wasmInstance = result.instance;
            }

            const loadTime = (performance.now() - startTime).toFixed(0);
            console.log(`✅ WASM loaded in ${loadTime}ms`);

            // Set up the wasmReady listener BEFORE running Go program
            const wasmReadyPromise = this._waitForWasmReady();

            // Run the Go program (this is async and starts the Go runtime)
            go.run(wasmInstance);

            // Wait for the wasmReady event from Go's main()
            await wasmReadyPromise;

            // Store reference to the global function
            this.generateFingerprint = window.generateFingerprint;
            this.wasmInstance = wasmInstance;

            // Initialize Web Audio API context
            this.audioContext = new (window.AudioContext || window.webkitAudioContext)();

            this.ready = true;
            this.loading = false;

            console.log('✅ WASM module ready');
            console.log(`   Sample Rate: ${this.audioContext.sampleRate} Hz`);

        } catch (error) {
            this.loading = false;
            console.error('❌ WASM initialization failed:', error);
            throw new Error(`Failed to initialize WASM: ${error.message}`);
        }
    }

    /**
     * Load the wasm_exec.js runtime
     * @private
     */
    async _loadWasmExec() {
        return new Promise((resolve, reject) => {
            if (window.Go) {
                resolve();
                return;
            }

            const script = document.createElement('script');
            script.src = 'wasm_exec.js';
            script.onload = () => {
                if (!window.Go) {
                    reject(new Error('wasm_exec.js loaded but Go is undefined'));
                    return;
                }
                console.log('✅ Go WASM runtime loaded');
                resolve();
            };
            script.onerror = () => reject(new Error('Failed to load wasm_exec.js'));
            document.head.appendChild(script);
        });
    }

    /**
     * Wait for the WASM module to signal it's ready
     * @private
     */
    async _waitForWasmReady() {
        return new Promise((resolve, reject) => {
            const timeout = setTimeout(() => {
                reject(new Error('WASM ready timeout (10s)'));
            }, 10000);

            const handler = () => {
                clearTimeout(timeout);
                window.removeEventListener('wasmReady', handler);
                resolve();
            };

            window.addEventListener('wasmReady', handler);
        });
    }

    /**
     * Process an audio file and extract fingerprint hashes
     * @param {File} file - Audio file from file input
     * @param {Function} progressCallback - Optional callback for progress updates
     * @returns {Promise<Array<{hash: number, anchorTime: number}>>}
     */
    async processAudioFile(file, progressCallback = null) {
        if (!this.ready) {
            throw new Error('WASM not initialized. Call init() first.');
        }

        if (!file) {
            throw new Error('No file provided');
        }

        console.log(`🎵 Processing audio file: ${file.name} (${(file.size / 1024 / 1024).toFixed(2)} MB)`);

        try {
            // Report progress: Reading file
            if (progressCallback) {
                progressCallback({ stage: 'reading', progress: 0 });
            }

            // Read file as ArrayBuffer
            const arrayBuffer = await file.arrayBuffer();

            // Report progress: Decoding
            if (progressCallback) {
                progressCallback({ stage: 'decoding', progress: 25 });
            }

            // Decode audio using Web Audio API
            const startDecode = performance.now();
            const audioBuffer = await this.audioContext.decodeAudioData(arrayBuffer.slice(0));
            const decodeTime = (performance.now() - startDecode).toFixed(0);

            console.log(`✅ Audio decoded in ${decodeTime}ms`);
            console.log(`   Duration: ${audioBuffer.duration.toFixed(1)}s`);
            console.log(`   Original Sample Rate: ${audioBuffer.sampleRate} Hz`);
            console.log(`   Channels: ${audioBuffer.numberOfChannels}`);

            // Report progress: Extracting samples
            if (progressCallback) {
                progressCallback({ stage: 'extracting', progress: 50 });
            }

            // Target sample rate must match the server's expected sample rate (11025 Hz)
            const TARGET_SAMPLE_RATE = 11025;

            let samples;
            let finalSampleRate;
            const channels = audioBuffer.numberOfChannels;

            // Check if we need to resample
            if (audioBuffer.sampleRate !== TARGET_SAMPLE_RATE) {
                console.log(`🔄 Resampling from ${audioBuffer.sampleRate} Hz to ${TARGET_SAMPLE_RATE} Hz...`);

                // Create an offline audio context for resampling
                const offlineContext = new OfflineAudioContext(
                    1, // mono output
                    Math.ceil(audioBuffer.duration * TARGET_SAMPLE_RATE),
                    TARGET_SAMPLE_RATE
                );

                // Create a buffer source
                const source = offlineContext.createBufferSource();
                source.buffer = audioBuffer;
                source.connect(offlineContext.destination);
                source.start(0);

                // Render the resampled audio
                const resampledBuffer = await offlineContext.startRendering();
                samples = Array.from(resampledBuffer.getChannelData(0));
                finalSampleRate = TARGET_SAMPLE_RATE;

                console.log(`✅ Resampled to ${TARGET_SAMPLE_RATE} Hz`);
                console.log(`   Resampled Samples: ${samples.length}`);
            } else if (channels === 1) {
                // Already at correct sample rate, mono audio
                samples = Array.from(audioBuffer.getChannelData(0));
                finalSampleRate = audioBuffer.sampleRate;
            } else {
                // Already at correct sample rate but stereo - need to convert to mono
                const left = audioBuffer.getChannelData(0);
                const right = audioBuffer.getChannelData(1);
                samples = new Array(left.length);
                for (let i = 0; i < left.length; i++) {
                    samples[i] = (left[i] + right[i]) / 2; // Average stereo to mono
                }
                finalSampleRate = audioBuffer.sampleRate;
            }

            console.log(`   Final Samples: ${samples.length}`);

            console.log(`   Final Samples: ${samples.length}`);

            // Report progress: Generating fingerprint
            if (progressCallback) {
                progressCallback({ stage: 'fingerprinting', progress: 75 });
            }

            // Call WASM fingerprint function with mono samples at correct sample rate
            const startFingerprint = performance.now();
            const result = this.generateFingerprint(samples, finalSampleRate, 1); // Always mono now
            const fingerprintTime = (performance.now() - startFingerprint).toFixed(0);

            // Check for errors
            if (result.error !== 0) {
                throw new Error(`Fingerprinting failed (error ${result.error}): ${result.data}`);
            }

            // Convert JavaScript array to regular array
            const hashes = Array.from(result.data);

            console.log(`✅ Generated ${hashes.length} hashes in ${fingerprintTime}ms`);
            console.log(`   Hashes per second: ${(hashes.length / audioBuffer.duration).toFixed(0)}`);

            // Report progress: Complete
            if (progressCallback) {
                progressCallback({ stage: 'complete', progress: 100 });
            }

            return hashes;

        } catch (error) {
            console.error('❌ Audio processing failed:', error);
            throw error;
        }
    }

    /**
     * Convert hash array to the format expected by the server API
     * @param {Array<{hash: number, anchorTime: number}>} hashes
     * @returns {Object} Map of hash -> anchorTime
     */
    hashesToServerFormat(hashes) {
        const hashMap = {};
        for (const { hash, anchorTime } of hashes) {
            // Ensure hash is treated as unsigned 32-bit integer
            // JavaScript numbers are 64-bit floats, but we need to ensure proper serialization
            const hashKey = hash >>> 0; // Convert to unsigned 32-bit integer

            // If multiple hashes have the same value, keep the first occurrence
            // (In practice, each hash should be unique in the query)
            if (!(hashKey in hashMap)) {
                hashMap[hashKey] = anchorTime;
            }
        }

        // Debug: Log a few sample hashes to verify format
        const sampleHashes = Object.keys(hashMap).slice(0, 5);
        console.log(`   Sample hash values: ${sampleHashes.join(', ')}`);

        return hashMap;
    }

    /**
     * Send hashes to the server for matching
     * @param {Array<{hash: number, anchorTime: number}>} hashes
     * @param {string} serverUrl - Base URL of the AcousticDNA server
     * @returns {Promise<Object>} Match results from server
     */
    async matchHashes(hashes, serverUrl = 'http://localhost:8080') {
        if (!this.ready) {
            throw new Error('WASM not initialized');
        }

        const hashMap = this.hashesToServerFormat(hashes);

        console.log(`🔍 Sending ${Object.keys(hashMap).length} unique hashes to server...`);

        try {
            const response = await fetch(`${serverUrl}/api/match/hashes`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ hashes: hashMap }),
            });

            if (!response.ok) {
                const errorData = await response.json().catch(() => ({}));
                throw new Error(errorData.message || `Server error: ${response.status}`);
            }

            const result = await response.json();
            console.log(`✅ Found ${result.count} match(es)`);

            return result;

        } catch (error) {
            console.error('❌ Server request failed:', error);
            throw error;
        }
    }

    /**
     * Complete workflow: process file and match against server
     * @param {File} file - Audio file
     * @param {string} serverUrl - Server URL
     * @param {Function} progressCallback - Progress callback
     * @returns {Promise<Object>} Match results
     */
    async processAndMatch(file, serverUrl = 'http://localhost:8080', progressCallback = null) {
        // Generate fingerprints
        const hashes = await this.processAudioFile(file, progressCallback);

        // Match against server
        const results = await this.matchHashes(hashes, serverUrl);

        return results;
    }
}

// Export singleton instance
export const wasmLoader = new WASMLoader();

// Also expose globally for easier console testing
if (typeof window !== 'undefined') {
    window.acousticDNA = wasmLoader;
}
