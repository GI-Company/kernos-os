import React, { useEffect, useState, useRef } from 'react';
import { kernel } from '../services/kernel';
import { Envelope } from '../types';
import { Activity, ArrowRight, Server, Globe, Power, Zap, Wifi, WifiOff, Trash2 } from 'lucide-react';

export const MonitorApp: React.FC = () => {
  const [traffic, setTraffic] = useState<Envelope[]>([]);
  const [isLive, setIsLive] = useState(kernel.isLive);
  const [wsUrl, setWsUrl] = useState('ws://localhost:8080/ws');
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Initial fetch
    setTraffic(kernel.getTrafficLog());

    // Subscribe to bus
    const unsub = kernel.subscribe((env) => {
      setTraffic(prev => [env, ...prev].slice(0, 50)); // Keep last 50 messages
    });
    
    // Poll for live status in case it changes externally (e.g. connection drop)
    const interval = setInterval(() => {
        if (kernel.isLive !== isLive) {
            setIsLive(kernel.isLive);
        }
    }, 500);

    return () => {
        unsub();
        clearInterval(interval);
    };
  }, [isLive]);

  const toggleConnection = () => {
      if (isLive) {
          kernel.disconnect();
          setIsLive(false);
      } else {
          kernel.connect(wsUrl);
          // We rely on the poll or callback to set isLive=true eventually
          // but we set it optimistically or wait for socket.onopen
          setTimeout(() => setIsLive(kernel.isLive), 100); 
      }
  };

  return (
    <div className="h-full bg-[#111115] text-gray-300 flex flex-col font-mono text-xs">
      {/* Control Panel */}
      <div className="p-4 border-b border-white/5 bg-white/5 space-y-4">
          <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                  <Activity size={16} className="text-orange-400" />
                  <span className="font-bold text-sm text-white">System Bus</span>
              </div>
              <div className={`flex items-center gap-2 px-2 py-1 rounded border ${isLive ? 'bg-green-500/10 border-green-500/30 text-green-400' : 'bg-red-500/10 border-red-500/30 text-red-400'}`}>
                  {isLive ? <Wifi size={12} /> : <WifiOff size={12} />}
                  <span className="uppercase tracking-wider font-bold text-[10px]">{isLive ? 'LIVE KERNEL' : 'SIMULATION'}</span>
              </div>
          </div>

          <div className="flex gap-2">
              <input 
                type="text" 
                value={wsUrl}
                onChange={(e) => setWsUrl(e.target.value)}
                className="flex-1 bg-[#050505] border border-white/10 rounded px-3 py-1.5 text-gray-400 focus:border-cyan-500 outline-none transition-colors"
                placeholder="ws://localhost:8080/ws"
                disabled={isLive}
              />
              <button 
                onClick={toggleConnection}
                className={`px-4 py-1.5 rounded font-medium transition-colors flex items-center gap-2 ${
                    isLive 
                    ? 'bg-red-600 hover:bg-red-500 text-white' 
                    : 'bg-green-600 hover:bg-green-500 text-white'
                }`}
              >
                <Power size={12} />
                {isLive ? 'Disconnect' : 'Connect'}
              </button>
          </div>
      </div>

      {/* Stats Panel */}
      <div className="grid grid-cols-2 border-b border-white/5 divide-x divide-white/5">
          <div className="p-3 flex items-center gap-3">
             <Server size={14} className="text-purple-400" />
             <div>
                 <div className="text-[10px] text-gray-500 uppercase">Latency</div>
                 <div className="text-lg font-bold text-white">{isLive ? '4ms' : '0ms'}</div>
             </div>
          </div>
          <div className="p-3 flex items-center gap-3">
             <Globe size={14} className="text-cyan-400" />
             <div>
                 <div className="text-[10px] text-gray-500 uppercase">Packets</div>
                 <div className="text-lg font-bold text-white">{traffic.length}</div>
             </div>
          </div>
      </div>

      {/* Traffic Log Header */}
      <div className="px-2 py-1 bg-white/5 border-b border-white/5 flex items-center justify-between text-[10px] text-gray-500 uppercase tracking-wider">
        <span>Event Stream</span>
        <button onClick={() => setTraffic([])} className="hover:text-white"><Trash2 size={10} /></button>
      </div>

      {/* Traffic Log */}
      <div className="flex-1 overflow-y-auto p-2 space-y-1" ref={scrollRef}>
          {traffic.map((env, i) => (
              <div key={i} className="group flex flex-col gap-1 p-2 hover:bg-white/5 rounded transition-colors border border-transparent hover:border-white/5">
                  <div className="flex items-center gap-2">
                      <div className="w-16 text-gray-600 text-[10px]">{env.time.split('T')[1]?.split('.')[0] || env.time}</div>
                      <div className="text-cyan-500 font-bold">{env.topic}</div>
                      <div className="text-purple-400 opacity-60">from: {env.from}</div>
                  </div>
                  <div className="text-gray-400 font-sans break-all pl-18 text-[10px] opacity-80 border-l border-white/10 pl-2 ml-2">
                      {JSON.stringify(env.payload)}
                  </div>
              </div>
          ))}
          {traffic.length === 0 && (
              <div className="p-8 text-center text-gray-600 italic">
                  No traffic on the bus...
              </div>
          )}
      </div>
    </div>
  );
};