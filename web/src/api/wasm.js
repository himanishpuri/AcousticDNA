// WASM loader for AcousticDNA fingerprinting

class WASMLoader {
    constructor() {
        this.ready = false;
        this.loading = false;
        this.generateFingerprint = null;
        this.wasmInstance = null;
        this.audioContext = null;
    }

    async init() {
        if (this.ready) {
            console.log('✅ WASM already initialized');
            return;
        }

        if (this.loading) {
            console.log('⏳ WASM initialization already in progress');
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
            await this._loadWasmExec();
            const go = new window.Go();
            const wasmPath = 'fingerprint.wasm';
            console.log(`📥 Loading WASM from ${wasmPath}...`);

            const startTime = performance.now();

            let wasmInstance;
            if (WebAssembly.instantiateStreaming) {
                const response = await fetch(wasmPath);
                const result = await WebAssembly.instantiateStreaming(response, go.importObject);
                wasmInstance = result.instance;
            } else {
                const response = await fetch(wasmPath);
                const buffer = await response.arrayBuffer();
                const result = await WebAssembly.instantiate(buffer, go.importObject);
                wasmInstance = result.instance;
            }

            const loadTime = (performance.now() - startTime).toFixed(0);
            console.log(`✅ WASM loaded in ${loadTime}ms`);

            go.run(wasmInstance);
            await this._waitForWasmReady();

            this.generateFingerprint = window.generateFingerprint;
            this.wasmInstance = wasmInstance;

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

    async processAudioFile(file, progressCallback = null) {
        if (!this.ready) {
            throw new Error('WASM not initialized. Call init() first.');
        }

        if (!file) {
            throw new Error('No file provided');
        }

        console.log(`🎵 Processing audio file: ${file.name} (${(file.size / 1024 / 1024).toFixed(2)} MB)`);

        try {
            if (progressCallback) {
                progressCallback({ stage: 'reading', progress: 0 });
            }

            const arrayBuffer = await file.arrayBuffer();

            if (progressCallback) {
                progressCallback({ stage: 'decoding', progress: 25 });
            }

            const startDecode = performance.now();
            const audioBuffer = await this.audioContext.decodeAudioData(arrayBuffer.slice(0));
            const decodeTime = (performance.now() - startDecode).toFixed(0);

            console.log(`✅ Audio decoded in ${decodeTime}ms`);
            console.log(`   Duration: ${audioBuffer.duration.toFixed(1)}s`);
            console.log(`   Sample Rate: ${audioBuffer.sampleRate} Hz`);
            console.log(`   Channels: ${audioBuffer.numberOfChannels}`);

            if (progressCallback) {
                progressCallback({ stage: 'extracting', progress: 50 });
            }

            let samples;
            const channels = audioBuffer.numberOfChannels;

            if (channels === 1) {
                samples = Array.from(audioBuffer.getChannelData(0));
            } else {
                const left = audioBuffer.getChannelData(0);
                const right = audioBuffer.getChannelData(1);
                samples = new Array(left.length * 2);
                for (let i = 0; i < left.length; i++) {
                    samples[i * 2] = left[i];
                    samples[i * 2 + 1] = right[i];
                }
            }

            console.log(`   Samples: ${samples.length}`);

            if (progressCallback) {
                progressCallback({ stage: 'fingerprinting', progress: 75 });
            }

            const startFingerprint = performance.now();
            const result = this.generateFingerprint(samples, audioBuffer.sampleRate, channels);
            const fingerprintTime = (performance.now() - startFingerprint).toFixed(0);

            if (result.error !== 0) {
                throw new Error(`Fingerprinting failed (error ${result.error}): ${result.data}`);
            }

            const hashes = Array.from(result.data);

            console.log(`✅ Generated ${hashes.length} hashes in ${fingerprintTime}ms`);
            console.log(`   Hashes per second: ${(hashes.length / audioBuffer.duration).toFixed(0)}`);

            if (progressCallback) {
                progressCallback({ stage: 'complete', progress: 100 });
            }

            return hashes;

        } catch (error) {
            console.error('❌ Audio processing failed:', error);
            throw error;
        }
    }

    hashesToServerFormat(hashes) {
        const hashMap = {};
        for (const { hash, anchorTime } of hashes) {
            if (!(hash in hashMap)) {
                hashMap[hash] = anchorTime;
            }
        }
        return hashMap;
    }

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

    async processAndMatch(file, serverUrl = 'http://localhost:8080', progressCallback = null) {
        const hashes = await this.processAudioFile(file, progressCallback);
        const results = await this.matchHashes(hashes, serverUrl);

        return results;
    }
}

export const wasmLoader = new WASMLoader();

if (typeof window !== 'undefined') {
    window.acousticDNA = wasmLoader;
}
