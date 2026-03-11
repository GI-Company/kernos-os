import { Envelope, FileNode } from '../types';

type BusListener = (envelope: Envelope) => void;

class P2PManager {
    private pc: RTCPeerConnection | null = null;
    private dc: RTCDataChannel | null = null;
    private kernel: Kernel;

    constructor(kernel: Kernel) {
        this.kernel = kernel;
        
        // Listen to the local bus for cross-network envelopes to forward over WebRTC
        this.kernel.subscribe((env: Envelope) => {
            if (env.to && env.to.includes('/') && this.dc && this.dc.readyState === 'open') {
                console.log("[P2P] Forwarding outbound envelope over WebRTC:", env.topic);
                this.dc.send(JSON.stringify(env));
            }
        });
    }

    public async startHosting(pin: string) {
        this.isHost = true;
        this.connectedPin = pin;
        this.pc = new RTCPeerConnection({ iceServers: [{ urls: 'stun:stun.l.google.com:19302' }] });
        this.dc = this.pc.createDataChannel('kernos-p2p');
        this.setupDataChannel();
        
        this.pc.onicecandidate = (event) => {
            if (event.candidate) {
                this.kernel.publish('p2p.signal', { targetPin: this.connectedPin, signalData: event.candidate });
            }
        };
        
        const offer = await this.pc.createOffer();
        await this.pc.setLocalDescription(offer);
        
        console.log(`[P2P Host] Offer generated. Signaling via PIN ${pin}...`);
        this.kernel.publish('p2p.signal', { targetPin: this.connectedPin, signalData: this.pc.localDescription });
        this.kernel.publish('p2p.host:start', {});
    }

    public async joinSession(pin: string) {
        this.isHost = false;
        this.connectedPin = pin;
        this.pc = new RTCPeerConnection({ iceServers: [{ urls: 'stun:stun.l.google.com:19302' }] });
        this.pc.ondatachannel = (event) => {
            this.dc = event.channel;
            this.setupDataChannel();
        };
        
        this.pc.onicecandidate = (event) => {
            if (event.candidate) {
                this.kernel.publish('p2p.signal', { targetPin: this.connectedPin, signalData: event.candidate });
            }
        };
        
        console.log(`[P2P Guest] Waiting for Host Offer on PIN ${pin}...`);
    }

    public async handleSignal(env: any) {
        if (!this.pc || env.topic !== 'p2p.signal:relay') return;
        
        const { targetPin, signalData, senderId } = env.payload;
        if (targetPin !== this.connectedPin) return; // Not for this session
        
        if (this.isHost) {
            // Host receives Answers and Guest ICE
            if (signalData.type === 'answer') {
                await this.pc.setRemoteDescription(new RTCSessionDescription(signalData));
                console.log("[P2P Host] Remote Answer accepted.");
            } else if (signalData.candidate) {
                await this.pc.addIceCandidate(new RTCIceCandidate(signalData));
            }
        } else {
            // Guest receives Offers and Host ICE
            if (signalData.type === 'offer') {
                await this.pc.setRemoteDescription(new RTCSessionDescription(signalData));
                const answer = await this.pc.createAnswer();
                await this.pc.setLocalDescription(answer);
                console.log("[P2P Guest] Offer accepted. Sending Answer.");
                this.kernel.publish('p2p.signal', { targetPin: this.connectedPin, signalData: answer });
            } else if (signalData.candidate) {
                await this.pc.addIceCandidate(new RTCIceCandidate(signalData));
            }
        }
    }

    public async acceptAnswer(answerSdp: string) {
        if (this.pc) {
            await this.pc.setRemoteDescription(new RTCSessionDescription({ type: 'answer', sdp: answerSdp }));
            console.log("[P2P Host] Guest answer accepted. Establishing connection...");
        }
    }
    
    public authenticate(pin: string) {
        if (this.dc && this.dc.readyState === 'open') {
            const authEnv: Envelope = {
                topic: 'p2p.auth',
                from: 'guest-node',
                payload: { pin },
                time: new Date().toISOString()
            };
            this.dc.send(JSON.stringify(authEnv));
        }
    }

    private setupDataChannel() {
        if (!this.dc) return;
        this.dc.onopen = () => console.log("[P2P] WebRTC Data Channel OPEN 🌐");
        this.dc.onclose = () => console.log("[P2P] WebRTC Data Channel CLOSED");
        this.dc.onmessage = (event) => {
            try {
                const env: Envelope = JSON.parse(event.data);
                console.log("[P2P] Received payload from remote peer:", env.topic);
                
                // System P2P topics (auth, topology) passthrough directly
                if (env.topic.startsWith('p2p.')) {
                     this.kernel.publish(env.topic, env.payload);
                } else {
                     // Regular subsystem tasks (vm.spawn, etc) must be wrapped in p2p.route 
                     // for the backend Gateway's zero-trust inspection
                     this.kernel.publish('p2p.route', {
                         topic: env.topic,
                         from: env.from || 'remote-peer',
                         to: env.to,
                         payload: env.payload
                     });
                }
            } catch (e) {
                console.error("[P2P] Parse error", e);
            }
        };
    }
}

/**
 * The Kernel acts as the bridge between the UI and the Execution Environment.
 * It supports two modes:
 * 1. SIMULATION (Default): Mocks responses using setTimeout.
 * 2. LIVE: Connects via WebSocket to a real Go server.
 */
class Kernel {
    private listeners: Set<BusListener> = new Set();
    private clientId: string = `client-${Math.random().toString(36).substring(2, 9)}`;
    private messageLog: Envelope[] = [];
    public p2p: P2PManager;

    // Connection State
    private socket: WebSocket | null = null;
    public isLive: boolean = false;
    private url: string = 'ws://localhost:8080/ws';

    // Mock VFS State (For Simulation Mode)
    private vfs: Record<string, FileNode> = {
        'root': { id: 'root', name: 'root', type: 'directory', children: ['home'], parentId: null },
        'home': { id: 'home', name: 'home', type: 'directory', children: ['welcome', 'docs', 'bin'], parentId: 'root' },
        'docs': { id: 'docs', name: 'docs', type: 'directory', children: ['specs'], parentId: 'home' },
        'specs': { id: 'specs', name: 'specs.txt', type: 'file', parentId: 'docs', content: 'Kernos OS Specifications...' },
        'welcome': { id: 'welcome', name: 'welcome.md', type: 'file', parentId: 'home', content: '# Welcome to Kernos\n\nThis is a browser-native OS.\nConnects to a Go Kernel via WebSockets.\n\nTry opening the Terminal and typing `help`.\n\nDouble click files in the File System to edit them.' },
        'bin': { id: 'bin', name: 'bin', type: 'directory', children: [], parentId: 'home' }
    };

    constructor() {
        this.p2p = new P2PManager(this);
        
        // Start Heartbeat
        setInterval(() => {
            if (!this.isLive) {
                this.publishInternal('system.status', { cpu: Math.random() * 20 + 10, memory: Math.random() * 40 + 20 });
            }
        }, 2000);
    }

    // --- Connection Management ---

    public connect(url: string = 'ws://localhost:8080/ws') {
        if (this.socket) this.socket.close();
        this.url = url;

        try {
            this.socket = new WebSocket(url);

            this.socket.onopen = () => {
                console.log('[Kernel] Connected to Real Backend');
                this.isLive = true;

                // -------------------------------------------------------------
                // 1. ZERO-TRUST AUTHENTICATION
                // Extract the token from the URL hash and authenticate immediately.
                // -------------------------------------------------------------
                const hashParts = window.location.hash.split('auth=');
                const token = hashParts.length > 1 ? hashParts[1] : '';

                const authEnv: Envelope = {
                    topic: 'sys.auth',
                    from: this.clientId,
                    payload: { token },
                    time: new Date().toISOString()
                };
                this.socket!.send(JSON.stringify(authEnv));

                // 2. Register this UI client
                const regEnv: Envelope = {
                    topic: 'sys.register',
                    from: this.clientId,
                    payload: { id: this.clientId, role: 'ui', name: 'Kernos Shell' },
                    time: new Date().toISOString()
                };
                this.socket!.send(JSON.stringify(regEnv));
                this.broadcast({ topic: 'system.connect', from: 'kernel', payload: { status: 'connected' }, time: new Date().toISOString() });
            };

            this.socket.onmessage = (event) => {
                try {
                    const envelope: Envelope = JSON.parse(event.data);
                    this.handleIncoming(envelope);
                } catch (e) {
                    console.error('Failed to parse envelope', e);
                }
            };

            this.socket.onclose = () => {
                console.log('[Kernel] Disconnected');
                this.isLive = false;
                this.socket = null;
                this.broadcast({ topic: 'system.disconnect', from: 'kernel', payload: { status: 'disconnected' }, time: new Date().toISOString() });
            };

            this.socket.onerror = (err) => {
                console.error('[Kernel] Socket Error', err);
            };

        } catch (e) {
            console.error("Connection failed", e);
        }
    }

    public disconnect() {
        if (this.socket) {
            this.socket.close();
            this.socket = null;
        }
        this.isLive = false;
    }

    // --- Bus Logic ---

    public getClientId() { return this.clientId; }

    public subscribe(listener: BusListener) {
        this.listeners.add(listener);
        return () => { this.listeners.delete(listener); };
    }

    public publish(topic: string, payload: any) {
        const envelope: Envelope = {
            topic,
            from: this.clientId,
            payload,
            time: new Date().toISOString()
        };

        // Log locally
        this.messageLog = [envelope, ...this.messageLog].slice(0, 100);

        // Decision: Send to Real Socket or Mock Kernel?
        if (this.isLive && this.socket && this.socket.readyState === WebSocket.OPEN) {
            this.socket.send(JSON.stringify(envelope));
        } else {
            // Fallback to Simulation
            this.processMockEnvelope(envelope);
        }
    }

    private handleIncoming(envelope: Envelope) {
        this.messageLog = [envelope, ...this.messageLog].slice(0, 100);
        
        // Let P2P Manager sniff signaling prior to UI broadcast
        if (envelope.topic === 'p2p.signal:relay') {
            this.p2p.handleSignal(envelope);
            return;
        }

        this.broadcast(envelope);
    }

    private broadcast(envelope: Envelope) {
        this.listeners.forEach(l => l(envelope));
    }

    private publishInternal(topic: string, payload: any) {
        const envelope: Envelope = { topic, from: 'kernel', payload, time: new Date().toISOString() };
        this.broadcast(envelope);
    }

    public getTrafficLog() { return this.messageLog; }

    /**
     * Send a targeted message to a specific agent via the WebSocket bus.
     */
    public sendToAgent(agentId: string, topic: string, payload: any) {
        const envelope: Envelope = {
            topic,
            from: this.clientId,
            to: agentId,
            payload,
            time: new Date().toISOString()
        };
        this.messageLog = [envelope, ...this.messageLog].slice(0, 100);
        if (this.isLive && this.socket && this.socket.readyState === WebSocket.OPEN) {
            this.socket.send(JSON.stringify(envelope));
        }
    }

    /**
     * Get a reference to the raw WebSocket (for advanced use).
     */
    public getSocket(): WebSocket | null {
        return this.socket;
    }


    // --- MOCK IMPLEMENTATION (Legacy/Fallback) ---

    private processMockEnvelope(envelope: Envelope) {
        this.broadcast(envelope); // Echo back to UI

        if (envelope.topic.startsWith('vm.')) this.handleMockVM(envelope);
        if (envelope.topic.startsWith('ai.')) this.handleMockAI(envelope);
        if (envelope.topic.startsWith('vfs:')) this.handleMockVFS(envelope);
        if (envelope.topic.startsWith('task.')) this.handleMockTask(envelope);
        if (envelope.topic.startsWith('pkg.')) this.handleMockPkg(envelope);
    }

    // ... (Keep existing Mock Handlers below, condensed for brevity) ...

    private handleMockVM(env: Envelope) {
        // (Existing VM Mock Logic - kept for offline mode)
        const { _request_id, cmd, args, cwd } = env.payload;
        setTimeout(() => {
            if (cmd === 'echo') {
                this.publishInternal('vm.stdout', { _request_id, text: args.join(' ') + '\n' });
            } else if (cmd === 'ls') {
                // Basic LS mock
                const files = Object.values(this.vfs).filter(f => f.parentId === (cwd || 'home')).map(f => f.name);
                this.publishInternal('vm.stdout', { _request_id, text: files.join('  ') + '\n' });
            } else {
                this.publishInternal('vm.stdout', { _request_id, text: `(SIMULATED) Executed: ${cmd}\n` });
            }
            this.publishInternal('vm.exit', { _request_id, code: 0 });
        }, 100);
    }

    private handleMockAI(env: Envelope) {
        if (env.topic !== 'ai.chat') return;
        setTimeout(() => this.publishInternal('ai.stream', { _request_id: env.payload._request_id, chunk: "I am a simulated AI running in the browser. " }), 500);
        setTimeout(() => this.publishInternal('ai.done', { _request_id: env.payload._request_id }), 1000);
    }

    private handleMockVFS(env: Envelope) {
        // Basic read/write mocks to keep app functional without backend
        const { _request_id, id, content, path } = env.payload;
        if (env.topic === 'vfs:read') {
            const file = this.vfs[id];
            this.publishInternal('vfs:read:resp', { _request_id, id, content: file ? file.content : '', name: file?.name || 'unknown' });
        }
        if (env.topic === 'vfs:list') {
            const target = path || 'home';
            const files = Object.values(this.vfs).filter(f => f.parentId === target);
            this.publishInternal('vfs:list:resp', { _request_id, files, path: target });
        }
        if (env.topic === 'vfs:write') {
            if (this.vfs[id]) this.vfs[id].content = content;
            this.publishInternal('vfs:write:ack', { _request_id, id });
        }
    }

    private handleMockTask(env: Envelope) {
        if (env.topic === 'task.run') {
            this.publishInternal('task.run:ack', { _request_id: env.payload._request_id, runId: 'sim-run-1' });
            setTimeout(() => this.publishInternal('task.done', { runId: 'sim-run-1' }), 2000);
        }
    }

    private handleMockPkg(env: Envelope) {
        if (env.topic === 'pkg.install') {
            this.publishInternal('pkg.install:ack', { _request_id: env.payload._request_id, runId: 'sim-pkg-1' });
            setTimeout(() => this.publishInternal('pkg.install:done', { pkgName: env.payload.pkgName }), 1500);
        }
    }
}

export const kernel = new Kernel();