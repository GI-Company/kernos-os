import React, { useEffect, useState, useRef } from 'react';
import { kernel } from '../services/kernel';
import { Envelope } from '../types';
import { Bot, Send, Loader2, ChevronDown, Zap } from 'lucide-react';

interface ChatMessage {
    id: string;
    role: 'user' | 'agent';
    content: string;
    agentId?: string;
    time: string;
}

interface AgentInfo {
    id: string;
    name: string;
    model: string;
    role: string;
}

export const AIChatApp: React.FC = () => {
    const [messages, setMessages] = useState<ChatMessage[]>([]);
    const [input, setInput] = useState('');
    const [agents, setAgents] = useState<AgentInfo[]>([]);
    const [selectedAgent, setSelectedAgent] = useState<string>('');
    const [isWaiting, setIsWaiting] = useState(false);
    const [showSelector, setShowSelector] = useState(false);
    const [directMode, setDirectMode] = useState(false);
    const [streamBuffer, setStreamBuffer] = useState('');
    const bottomRef = useRef<HTMLDivElement>(null);
    const agentCheckTimer = useRef<ReturnType<typeof setTimeout>>();

    useEffect(() => {
        bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [messages, streamBuffer]);

    // After 3 seconds, if no agents connected, switch to direct LM mode
    useEffect(() => {
        agentCheckTimer.current = setTimeout(() => {
            if (agents.length === 0) {
                setDirectMode(true);
            }
        }, 3000);
        return () => { if (agentCheckTimer.current) clearTimeout(agentCheckTimer.current); };
    }, []);

    useEffect(() => {
        const unsub = kernel.subscribe((env: Envelope) => {
            // Track agent list
            if (env.topic === 'sys.client_list') {
                const payload = env.payload as any;
                const clients = (payload?.clients || []) as AgentInfo[];
                const agentClients = clients.filter((c: AgentInfo) => c.role === 'agent');
                setAgents(agentClients);
                if (agentClients.length > 0) {
                    setDirectMode(false);
                    if (!selectedAgent) {
                        setSelectedAgent(agentClients[0].id);
                    }
                }
            }

            // Receive chat replies (agent-targeted mode)
            if (env.topic === 'agent.chat:reply') {
                const payload = env.payload as any;
                const reply = payload?.reply || '(no response)';
                setStreamBuffer('');
                setMessages(prev => [...prev, {
                    id: Math.random().toString(36),
                    role: 'agent',
                    content: reply,
                    agentId: env.from,
                    time: new Date().toLocaleTimeString()
                }]);
                setIsWaiting(false);
            }

            // Agent streaming
            if (env.topic === 'agent.chat:stream') {
                const payload = env.payload as any;
                setStreamBuffer(prev => prev + (payload.chunk || ''));
                setIsWaiting(false);
            }

            // Streaming responses (direct LM mode)
            if (env.topic === 'ai.stream') {
                setStreamBuffer(prev => prev + (env.payload.chunk || ''));
                setIsWaiting(false);
            }

            if (env.topic === 'ai.done') {
                // Commit the stream buffer as a complete message
                setStreamBuffer(prev => {
                    if (prev) {
                        setMessages(msgs => [...msgs, {
                            id: Math.random().toString(36),
                            role: 'agent',
                            content: prev,
                            agentId: 'kernos-lm',
                            time: new Date().toLocaleTimeString()
                        }]);
                    }
                    return '';
                });
                setIsWaiting(false);
            }
        });
        return unsub;
    }, [selectedAgent]);

    const sendMessage = () => {
        const msg = input.trim();
        if (!msg) return;
        if (!directMode && !selectedAgent) return;

        setMessages(prev => [...prev, {
            id: Math.random().toString(36),
            role: 'user',
            content: msg,
            time: new Date().toLocaleTimeString()
        }]);

        if (directMode) {
            // Direct to kernel's embedded LM
            const reqId = Math.random().toString(36).substring(7);
            kernel.publish('ai.chat', { _request_id: reqId, prompt: msg });
        } else {
            // Targeted agent message
            kernel.sendToAgent(selectedAgent, 'agent.chat', { msg });
        }

        setInput('');
        setIsWaiting(true);
        setStreamBuffer('');
    };

    const currentAgent = agents.find(a => a.id === selectedAgent);

    return (
        <div className="h-full flex flex-col bg-[#0a0a0f] text-gray-200">
            {/* Header */}
            <div className="p-3 border-b border-white/5 bg-white/[0.03] flex items-center justify-between">
                <div className="flex items-center gap-2">
                    <Bot size={16} className="text-purple-400" />
                    <span className="text-sm font-medium text-white">AI Chat</span>
                    {directMode && (
                        <span className="text-[10px] px-2 py-0.5 rounded-full bg-cyan-500/10 border border-cyan-500/20 text-cyan-400 flex items-center gap-1">
                            <Zap size={8} /> Direct LM
                        </span>
                    )}
                </div>
                {!directMode ? (
                    <div className="relative">
                        <button
                            onClick={() => setShowSelector(!showSelector)}
                            className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/5 border border-white/10 text-xs hover:bg-white/10 transition-colors"
                        >
                            <div className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
                            <span>{currentAgent?.name || currentAgent?.id || 'No agent'}</span>
                            <ChevronDown size={12} />
                        </button>
                        {showSelector && agents.length > 0 && (
                            <div className="absolute right-0 top-full mt-1 bg-[#15151a] border border-white/10 rounded-lg shadow-xl z-50 min-w-[220px] overflow-hidden">
                                {agents.map(agent => (
                                    <button
                                        key={agent.id}
                                        onClick={() => { setSelectedAgent(agent.id); setShowSelector(false); }}
                                        className={`w-full text-left px-4 py-3 text-xs hover:bg-white/5 transition-colors border-b border-white/5 last:border-0 ${agent.id === selectedAgent ? 'bg-purple-500/10 text-purple-300' : 'text-gray-400'
                                            }`}
                                    >
                                        <div className="font-medium text-white">{agent.name || agent.id}</div>
                                        <div className="text-[10px] text-gray-500 mt-0.5 font-mono">{agent.model}</div>
                                    </button>
                                ))}
                            </div>
                        )}
                    </div>
                ) : (
                    <button
                        onClick={() => setDirectMode(false)}
                        className="text-[10px] px-2 py-1 rounded bg-white/5 text-gray-500 hover:text-white transition-colors"
                    >
                        Switch to Agent Mode
                    </button>
                )}
            </div>

            {/* Messages */}
            <div className="flex-1 overflow-y-auto p-4 space-y-4">
                {messages.length === 0 && !streamBuffer && (
                    <div className="flex flex-col items-center justify-center h-full text-center opacity-40">
                        <Bot size={40} className="text-purple-400 mb-3" />
                        <p className="text-sm text-gray-500">
                            {directMode
                                ? 'Chat directly with the Kernos LM engine.'
                                : 'Send a message to talk to a Kernos agent.'}
                        </p>
                        <p className="text-xs text-gray-600 mt-1">
                            {directMode
                                ? 'Your messages go to the embedded AI model.'
                                : 'Select an agent from the dropdown above.'}
                        </p>
                    </div>
                )}
                {messages.map(msg => (
                    <div key={msg.id} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                        <div className={`max-w-[80%] rounded-xl px-4 py-3 text-sm leading-relaxed ${msg.role === 'user'
                            ? 'bg-purple-600/20 border border-purple-500/20 text-gray-200'
                            : 'bg-white/5 border border-white/5 text-gray-300'
                            }`}>
                            {msg.role === 'agent' && (
                                <div className="text-[10px] text-purple-400 mb-1 font-mono">{msg.agentId}</div>
                            )}
                            <div className="whitespace-pre-wrap">{msg.content}</div>
                            <div className="text-[10px] text-gray-600 mt-2">{msg.time}</div>
                        </div>
                    </div>
                ))}
                {/* Live streaming buffer */}
                {streamBuffer && (
                    <div className="flex justify-start">
                        <div className="max-w-[80%] rounded-xl px-4 py-3 text-sm leading-relaxed bg-white/5 border border-white/5 text-gray-300">
                            <div className="text-[10px] text-cyan-400 mb-1 font-mono">{directMode ? 'kernos-lm' : (currentAgent?.id || 'agent')}</div>
                            <div className="whitespace-pre-wrap">{streamBuffer}<span className="animate-pulse">▊</span></div>
                        </div>
                    </div>
                )}
                {isWaiting && !streamBuffer && (
                    <div className="flex justify-start">
                        <div className="bg-white/5 border border-white/5 rounded-xl px-4 py-3 flex items-center gap-2 text-gray-500 text-sm">
                            <Loader2 size={14} className="animate-spin" />
                            <span>{directMode ? 'Kernos LM' : currentAgent?.name || 'Agent'} is thinking...</span>
                        </div>
                    </div>
                )}
                <div ref={bottomRef} />
            </div>

            {/* Input */}
            <div className="p-3 border-t border-white/5 bg-white/[0.02]">
                <div className="flex gap-2">
                    <input
                        type="text"
                        value={input}
                        onChange={e => setInput(e.target.value)}
                        onKeyDown={e => e.key === 'Enter' && sendMessage()}
                        placeholder={directMode ? 'Ask the Kernos AI anything...' : `Message ${currentAgent?.name || 'agent'}...`}
                        className="flex-1 bg-white/5 border border-white/10 rounded-lg px-4 py-2.5 text-sm text-white placeholder-gray-600 outline-none focus:border-purple-500/50 transition-colors"
                    />
                    <button
                        onClick={sendMessage}
                        disabled={!input.trim() || (!directMode && !selectedAgent)}
                        className="px-4 py-2.5 bg-purple-600 hover:bg-purple-500 disabled:bg-gray-800 disabled:text-gray-600 rounded-lg text-sm font-medium transition-colors flex items-center gap-2"
                    >
                        <Send size={14} />
                    </button>
                </div>
            </div>
        </div>
    );
};
