import React, { useState, useEffect } from 'react';
import { kernel } from '../services/kernel';
import { Envelope } from '../types';
import { FolderGit2, Search, FileCode2, Loader2, Workflow } from 'lucide-react';

interface VFSNode {
    id: string;
    name: string;
    type: string;
    similarity: number;
    content: string;
}

export const SemanticVFSApp: React.FC = () => {
    const [query, setQuery] = useState('');
    const [isSearching, setIsSearching] = useState(false);
    const [results, setResults] = useState<VFSNode[]>([]);
    const [selectedNode, setSelectedNode] = useState<VFSNode | null>(null);

    useEffect(() => {
        const unsub = kernel.subscribe((env: Envelope) => {
            if (env.topic === 'vfs:semantic:result') {
                const payload = env.payload as any;
                setResults(payload.results || []);
                setIsSearching(false);
            }
        });
        return unsub;
    }, []);

    const handleSearch = (e: React.FormEvent) => {
        e.preventDefault();
        if (!query.trim()) return;

        setIsSearching(true);
        setResults([]);
        setSelectedNode(null);

        kernel.publish('vfs:semantic', { query });
        if (kernel.isLive && (kernel as any).socket) {
            (kernel as any).socket.send(JSON.stringify({
                topic: 'vfs:semantic',
                from: kernel.getClientId(),
                payload: { query },
                time: new Date().toISOString()
            }));
        }
    };

    return (
        <div className="h-full flex flex-col bg-[#0c0c10] text-gray-300 font-sans">
            {/* Header */}
            <div className="p-4 border-b border-white/5 bg-white/5 flex items-center justify-between">
                <div className="flex items-center gap-2">
                    <FolderGit2 size={16} className="text-pink-400" />
                    <span className="text-sm font-bold text-white">Semantic VFS</span>
                </div>
                <div className="flex items-center gap-2 px-2 py-1 flex-1 max-w-md mx-4">
                    <form onSubmit={handleSearch} className="w-full relative">
                        <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                        <input
                            type="text"
                            value={query}
                            onChange={e => setQuery(e.target.value)}
                            placeholder="Query semantic path (e.g. 'authentication handlers')"
                            className="w-full bg-black/40 border border-white/10 rounded-full pl-9 pr-4 py-1.5 text-xs text-white placeholder-gray-600 focus:outline-none focus:border-pink-500/50 focus:bg-black/60 transition-all font-mono"
                        />
                    </form>
                </div>
                <div className="text-[10px] text-gray-500 font-mono flex items-center gap-1">
                    <Workflow size={12} className="text-gray-400" />
                    VectorDB
                </div>
            </div>

            <div className="flex-1 flex overflow-hidden">
                {/* Synthetic File Tree */}
                <div className="w-1/3 border-r border-white/5 overflow-y-auto bg-black/20 p-2">
                    {isSearching ? (
                        <div className="flex flex-col items-center justify-center p-8 text-gray-500 space-y-3">
                            <Loader2 size={24} className="animate-spin text-pink-500" />
                            <div className="text-xs font-mono">Generating Synthetic Path...</div>
                        </div>
                    ) : results.length === 0 ? (
                        <div className="p-4 text-xs text-gray-500 text-center flex flex-col items-center justify-center h-full opacity-50">
                            <FolderGit2 size={32} className="mb-2 text-gray-600" />
                            Query the vector space to generate files.
                        </div>
                    ) : (
                        <div className="space-y-1">
                            <div className="px-2 py-1 text-[10px] uppercase font-bold tracking-wider text-pink-500/50 mb-2">
                                /vfs/semantic/{query.replace(/\s+/g, '_')}
                            </div>
                            {results.map((node, i) => (
                                <button
                                    key={node.id}
                                    onClick={() => setSelectedNode(node)}
                                    className={`w-full text-left px-3 py-2 rounded flex items-center gap-3 text-xs font-mono group transition-colors ${selectedNode?.id === node.id ? 'bg-pink-500/20 text-pink-300' : 'hover:bg-white/5 text-gray-400 hover:text-gray-200'
                                        }`}
                                >
                                    <FileCode2 size={12} className={selectedNode?.id === node.id ? 'text-pink-400' : 'text-gray-500 group-hover:text-gray-400'} />
                                    <div className="flex-1 min-w-0">
                                        <div className="truncate">{node.name.split('/').pop()}</div>
                                        <div className="text-[9px] opacity-50 mt-0.5 max-w-[150px] truncate">{node.id}</div>
                                    </div>
                                    <div className={`text-[9px] px-1.5 py-0.5 rounded ${node.similarity > 0.8 ? 'bg-green-500/20 text-green-400' :
                                            node.similarity > 0.6 ? 'bg-yellow-500/20 text-yellow-400' :
                                                'bg-gray-800 text-gray-500'
                                        }`}>
                                        {(node.similarity * 100).toFixed(0)}%
                                    </div>
                                </button>
                            ))}
                        </div>
                    )}
                </div>

                {/* Content Viewer */}
                <div className="flex-1 bg-[#0a0a0f] overflow-y-auto">
                    {selectedNode ? (
                        <div className="p-4">
                            <div className="mb-4 text-xs font-mono text-gray-500 pb-2 border-b border-white/5 flex items-center justify-between">
                                <span>{selectedNode.name}</span>
                                <span className="text-pink-400/50">Synthetic Vector Node</span>
                            </div>
                            <pre className="text-xs font-mono text-gray-300 whitespace-pre-wrap font-medium leading-relaxed">
                                {selectedNode.content}
                            </pre>
                        </div>
                    ) : (
                        <div className="h-full flex items-center justify-center text-xs text-gray-600 font-mono">
                            Select a synthetic file to view contents.
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
};
