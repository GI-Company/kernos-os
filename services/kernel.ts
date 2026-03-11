import { Envelope, FileNode } from '../types';

type BusListener = (envelope: Envelope) => void;

class P2PManager {
    private pc: RTCPeerConnection | null = null;
    private dc: RTCDataChannel | null = null;
    private kernel: Kernel;
    private isHost: boolean = false;
    private connectedPin: string = '';

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
        if (targetPin !== this.connectedPin) return;
        
        if (this.isHost) {
            if (signalData.type === 'answer') {
                await this.pc.setRemoteDescription(new RTCSessionDescription(signalData));
                console.log("[P2P Host] Remote Answer accepted.");
            } else if (signalData.candidate) {
                await this.pc.addIceCandidate(new RTCIceCandidate(signalData));
            }
        } else {
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
                
                if (env.topic.startsWith('p2p.')) {
                     this.kernel.publish(env.topic, env.payload);
                } else {
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
 * The Kernel bridges the UI and the Go Execution Environment.
 * It auto-connects to the WebSocket backend on construction.
 * All communication flows through the real backend — no simulation mode.
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
    private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    constructor() {
        this.p2p = new P2PManager(this);
        // Auto-connect to the real backend
        this.connect();
    }

    // --- Connection Management ---

    public connect(url?: string) {
        if (url) this.url = url;
        if (this.socket) this.socket.close();

        try {
            this.socket = new WebSocket(this.url);

            this.socket.onopen = () => {
                console.log('[Kernel] ✅ Connected to Real Backend');
                this.isLive = true;
                if (this.reconnectTimer) {
                    clearTimeout(this.reconnectTimer);
                    this.reconnectTimer = null;
                }

                // Zero-Trust Authentication
                const hashParts = window.location.hash.split('auth=');
                const token = hashParts.length > 1 ? hashParts[1] : '';

                const authEnv: Envelope = {
                    topic: 'sys.auth',
                    from: this.clientId,
                    payload: { token },
                    time: new Date().toISOString()
                };
                this.socket!.send(JSON.stringify(authEnv));

                // Register this UI client
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
                console.log('[Kernel] ❌ Disconnected from backend');
                this.isLive = false;
                this.socket = null;
                this.broadcast({ topic: 'system.disconnect', from: 'kernel', payload: { status: 'disconnected' }, time: new Date().toISOString() });
                
                // Auto-reconnect after 3 seconds
                if (!this.reconnectTimer) {
                    this.reconnectTimer = setTimeout(() => {
                        console.log('[Kernel] 🔄 Attempting reconnect...');
                        this.connect();
                    }, 3000);
                }
            };

            this.socket.onerror = (err) => {
                console.warn('[Kernel] Socket error — backend may not be running. Will retry in 3s.', err);
            };

        } catch (e) {
            console.error("Connection failed", e);
            // Retry after 3 seconds
            if (!this.reconnectTimer) {
                this.reconnectTimer = setTimeout(() => this.connect(), 3000);
            }
        }
    }

    public disconnect() {
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
            this.reconnectTimer = null;
        }
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
        this.messageLog = [envelope, ...this.messageLog].slice(0, 200);

        if (this.isLive && this.socket && this.socket.readyState === WebSocket.OPEN) {
            this.socket.send(JSON.stringify(envelope));
        } else {
            // Backend is not connected — broadcast locally for UI echo only
            console.warn(`[Kernel] Bus offline — "${topic}" broadcast locally only`);
            this.broadcast(envelope);
        }
    }

    private handleIncoming(envelope: Envelope) {
        this.messageLog = [envelope, ...this.messageLog].slice(0, 200);
        
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
        this.messageLog = [envelope, ...this.messageLog].slice(0, 200);
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
}

export const kernel = new Kernel();