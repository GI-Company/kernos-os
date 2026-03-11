import React, { useState, useEffect } from 'react';
import { kernel } from '../../services/kernel';
import { Network, Server, Share2, Globe, KeyRound } from 'lucide-react';

export const P2PPortal: React.FC = () => {
    const [pin, setPin] = useState<string>('');
    const [mode, setMode] = useState<'idle' | 'host' | 'guest'>('idle');
    const [logs, setLogs] = useState<string[]>([]);
    
    // A simple log sink to capture P2P events from the class internals for the UI
    useEffect(() => {
        const _log = console.log;
        console.log = function(...args) {
             if (typeof args[0] === 'string' && args[0].includes('[P2P')) {
                 setLogs(prev => [...prev, args.join(' ')].slice(-10));
             }
             _log.apply(console, args);
        };
        return () => { console.log = _log; };
    }, []);

    const handleHost = () => {
        const newPin = Math.floor(1000 + Math.random() * 9000).toString();
        setPin(newPin);
        setMode('host');
        kernel.p2p.startHosting(newPin);
    };

    const handleJoin = async () => {
        if (!pin || pin.length !== 4) {
            setLogs(prev => [...prev, '[P2P Error] PIN must be 4 digits']);
            return;
        }
        setMode('guest');
        await kernel.p2p.joinSession(pin);
    };

    return (
        <div className="flex flex-col h-full bg-[#111111] text-white p-6 font-mono overflow-auto">
            <div className="flex items-center gap-3 mb-6 pb-4 border-b border-white/10">
                <Globe className="text-cyan-400" size={24} />
                <h2 className="text-xl font-bold tracking-tight">Zero-Trust P2P Portal</h2>
            </div>
            
            <p className="text-sm text-white/50 mb-8 leading-relaxed">
                Connect two Kernos OS instances directly via WebRTC Data Channels. 
                All traffic bypasses the central server. The backend acts only as a secure PIN-based signaling relay.
            </p>

            {mode === 'idle' ? (
                <div className="grid grid-cols-2 gap-6">
                    <div className="bg-[#1a1a1a] p-6 rounded border border-white/5 hover:border-cyan-500/30 transition-colors flex flex-col h-full">
                        <Server className="text-emerald-400 mb-4" size={32} />
                        <h3 className="text-lg font-bold mb-2">Host Session</h3>
                        <p className="text-xs text-white/40 mb-6 flex-1">
                            Generate a secure Session PIN. Share this PIN with a peer to allow them to connect and pool local agents.
                        </p>
                        <button 
                            onClick={handleHost}
                            className="bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 border border-emerald-500/30 rounded py-2 px-4 transition-all w-full font-bold"
                        >
                            Start Hosting
                        </button>
                    </div>

                    <div className="bg-[#1a1a1a] p-6 rounded border border-white/5 hover:border-blue-500/30 transition-colors flex flex-col h-full">
                        <Share2 className="text-blue-400 mb-4" size={32} />
                        <h3 className="text-lg font-bold mb-2">Join Session</h3>
                        <p className="text-xs text-white/40 mb-6 flex-1">
                            Enter a 4-digit PIN provided by a Host to securely negotiate a direct connection.
                        </p>
                        <div className="flex gap-2 mt-auto">
                            <div className="flex-1 relative">
                                <KeyRound className="absolute left-3 top-1/2 -translate-y-1/2 text-white/30" size={14} />
                                <input 
                                    type="text" 
                                    maxLength={4}
                                    placeholder="0000"
                                    value={pin}
                                    onChange={e => setPin(e.target.value.replace(/\D/g, ''))}
                                    className="w-full bg-black border border-white/10 rounded py-2 pl-9 pr-4 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all font-mono tracking-widest text-center"
                                />
                            </div>
                            <button 
                                onClick={handleJoin}
                                className="bg-blue-500/10 text-blue-400 hover:bg-blue-500/20 border border-blue-500/30 rounded py-2 px-4 transition-all font-bold"
                            >
                                Connect
                            </button>
                        </div>
                    </div>
                </div>
            ) : (
                <div className="flex flex-col flex-1">
                    <div className="flex items-center justify-between bg-black/50 p-4 rounded border border-white/5 mb-6">
                        <div className="flex items-center gap-3">
                            <Network className={mode === 'host' ? 'text-emerald-400' : 'text-blue-400'} size={20} />
                            <span className="text-sm font-bold opacity-75 uppercase tracking-widest text-white/70">
                                {mode} Mode Active
                            </span>
                        </div>
                        <div className="flex items-center gap-3">
                             <span className="text-xs text-white/40">Session PIN</span>
                             <span className="bg-white/10 px-3 py-1 rounded text-cyan-300 font-bold tracking-widest font-mono border border-cyan-500/30 shadow-[0_0_10px_rgba(34,211,238,0.2)]">
                                {pin}
                             </span>
                        </div>
                    </div>
                    
                    <h3 className="text-xs font-bold text-white/30 uppercase tracking-widest mb-2 mt-2 border-b border-white/5 pb-2">WebRTC Negotiation Log</h3>
                    <div className="bg-black/80 rounded border border-white/5 p-4 flex-1 font-mono text-xs text-green-400/80 leading-relaxed overflow-y-auto">
                        <div className="flex flex-col gap-1">
                            {logs.length === 0 && <span className="opacity-50 italic">Waiting for ICE and SDP signals...</span>}
                            {logs.map((log, i) => (
                                <div key={i} className="whitespace-pre-wrap break-words">{log}</div>
                            ))}
                        </div>
                    </div>
                    
                    <button 
                        onClick={() => { setMode('idle'); setPin(''); setLogs([]); kernel.p2p.startHosting(''); }} // Reset
                        className="mt-6 bg-red-500/10 text-red-500 hover:bg-red-500/20 border border-red-500/30 rounded py-2 px-4 transition-all font-bold w-full"
                    >
                        Disconnect & Abort
                    </button>
                </div>
            )}
        </div>
    );
};
