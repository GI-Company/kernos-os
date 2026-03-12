import React, { useState, useEffect } from 'react';
import { kernel } from '../services/kernel';
import { Envelope } from '../types';
import { Package, Download, Check, Loader, Box, Search, RefreshCw, Trash2 } from 'lucide-react';

interface PkgInfo {
    name: string;
    desc: string;
    version: string;
    installed: boolean;
}

const FALLBACK_PACKAGES: PkgInfo[] = [
    { name: 'python', desc: 'Python 3.12 Interpreter', version: '3.12.2', installed: false },
    { name: 'node', desc: 'Node.js Runtime', version: '20.11.1', installed: false },
    { name: 'go', desc: 'Go Programming Language', version: '1.22.0', installed: false },
    { name: 'rustc', desc: 'Rust Compiler & Cargo', version: '1.76.0', installed: false },
    { name: 'deno', desc: 'Deno Runtime (TypeScript/JS)', version: '1.40.5', installed: false },
    { name: 'ffmpeg', desc: 'Media Processing Suite', version: '6.0.0', installed: false },
    { name: 'sqlite', desc: 'SQL Database Engine', version: '0.21.6', installed: false },
    { name: 'ripgrep', desc: 'Ultra-fast recursive search (rg)', version: '14.1.0', installed: false },
    { name: 'jq', desc: 'Command-line JSON Processor', version: '1.7.1', installed: false },
];

export const PackageManagerApp: React.FC = () => {
    const [packages, setPackages] = useState<PkgInfo[]>([]);
    const [installing, setInstalling] = useState<string | null>(null);
    const [uninstalling, setUninstalling] = useState<string | null>(null);
    const [progress, setProgress] = useState('');
    const [filter, setFilter] = useState('');
    const [loading, setLoading] = useState(true);

    const fetchPackages = () => {
        setLoading(true);
        kernel.publish('pkg.list', { _request_id: Math.random().toString() });
        // Fallback to static list after 2 seconds if no response
        setTimeout(() => {
            setPackages(prev => prev.length === 0 ? FALLBACK_PACKAGES : prev);
            setLoading(false);
        }, 2000);
    };

    useEffect(() => {
        fetchPackages();

        const unsub = kernel.subscribe((env: Envelope) => {
            if (env.topic === 'pkg.list:resp') {
                const pkgs = env.payload?.packages || [];
                if (pkgs.length > 0) {
                    setPackages(pkgs);
                    setLoading(false);
                }
            }
            if (env.topic === 'pkg.install:done') {
                const { pkgName } = env.payload;
                setPackages(prev => prev.map(p => p.name === pkgName ? { ...p, installed: true } : p));
                setInstalling(null);
                setProgress('');
            }
            if (env.topic === 'pkg.uninstall:done') {
                const { pkgName, error } = env.payload;
                if (!error) {
                    setPackages(prev => prev.map(p => p.name === pkgName ? { ...p, installed: false } : p));
                }
                setUninstalling(null);
            }
            if (env.topic === 'task.event' && installing && env.payload.runId?.startsWith('pkg')) {
                const { step, status } = env.payload;
                if (status === 'running') {
                    setProgress(step);
                }
            }
        });
        return unsub;
    }, [installing]);

    const handleInstall = (pkg: PkgInfo) => {
        if (pkg.installed || installing || uninstalling) return;
        setInstalling(pkg.name);
        kernel.publish('pkg.install', { _request_id: Math.random().toString(), pkgName: pkg.name });
    };

    const handleUninstall = (pkg: PkgInfo) => {
        if (!pkg.installed || installing || uninstalling) return;
        setUninstalling(pkg.name);
        kernel.publish('pkg.uninstall', { pkgName: pkg.name });
    };

    const filtered = filter
        ? packages.filter(p => p.name.toLowerCase().includes(filter.toLowerCase()) || p.desc.toLowerCase().includes(filter.toLowerCase()))
        : packages;

    const installedCount = packages.filter(p => p.installed).length;

    return (
        <div className="h-full bg-[#0a0a0f] text-white flex flex-col">
            <div className="p-4 border-b border-white/5 bg-white/[0.03]">
                <div className="flex items-center gap-3 mb-3">
                    <Package size={20} className="text-cyan-400" />
                    <div>
                        <h2 className="text-sm font-bold">Kernos Package Manager</h2>
                        <p className="text-[10px] text-gray-500">
                            {loading ? 'Querying registry...' : `${packages.length} packages available · ${installedCount} installed`}
                        </p>
                    </div>
                    <div className="flex-1" />
                    <button onClick={fetchPackages} className="p-1.5 hover:bg-white/10 rounded text-gray-400" title="Refresh">
                        <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
                    </button>
                </div>
                <div className="flex items-center gap-2 bg-black/40 border border-white/10 rounded-lg px-3">
                    <Search size={12} className="text-gray-600" />
                    <input
                        type="text"
                        value={filter}
                        onChange={e => setFilter(e.target.value)}
                        placeholder="Search packages..."
                        className="flex-1 bg-transparent py-1.5 text-gray-300 placeholder-gray-600 outline-none text-xs"
                    />
                </div>
            </div>

            <div className="flex-1 overflow-y-auto p-4 space-y-2">
                {filtered.map(pkg => (
                    <div key={pkg.name} className="flex items-center justify-between p-3 rounded-lg bg-white/[0.03] border border-white/5 hover:border-white/10 transition-colors">
                        <div className="flex items-center gap-4">
                            <div className="p-2 bg-black/40 rounded-lg text-gray-400">
                                <Box size={20} />
                            </div>
                            <div>
                                <div className="font-bold text-sm flex items-center gap-2">
                                    {pkg.name}
                                    <span className="text-[10px] bg-white/10 px-1.5 py-0.5 rounded text-gray-400">v{pkg.version}</span>
                                </div>
                                <div className="text-xs text-gray-500">{pkg.desc}</div>
                            </div>
                        </div>

                        <div className="flex items-center gap-2">
                            {installing === pkg.name ? (
                                <div className="flex flex-col items-end">
                                    <div className="flex items-center gap-2 text-cyan-400 text-xs">
                                        <Loader size={12} className="animate-spin" />
                                        <span>Installing...</span>
                                    </div>
                                    <span className="text-[10px] text-gray-600 font-mono mt-1 max-w-[180px] truncate">{progress}</span>
                                </div>
                            ) : uninstalling === pkg.name ? (
                                <div className="flex items-center gap-2 text-red-400 text-xs">
                                    <Loader size={12} className="animate-spin" />
                                    <span>Removing...</span>
                                </div>
                            ) : pkg.installed ? (
                                <div className="flex items-center gap-2">
                                    <div className="flex items-center gap-1 text-green-500 text-xs px-3 py-1.5 bg-green-500/10 rounded-lg">
                                        <Check size={14} />
                                        <span>Installed</span>
                                    </div>
                                    <button
                                        onClick={() => handleUninstall(pkg)}
                                        disabled={!!installing || !!uninstalling}
                                        className="p-1.5 text-gray-600 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors disabled:opacity-30"
                                        title={`Uninstall ${pkg.name}`}
                                    >
                                        <Trash2 size={14} />
                                    </button>
                                </div>
                            ) : (
                                <button 
                                    onClick={() => handleInstall(pkg)}
                                    disabled={!!installing || !!uninstalling}
                                    className="flex items-center gap-1.5 text-xs bg-cyan-600 hover:bg-cyan-500 disabled:opacity-50 disabled:cursor-not-allowed text-white px-3 py-1.5 rounded-lg transition-colors"
                                >
                                    <Download size={14} />
                                    Get
                                </button>
                            )}
                        </div>
                    </div>
                ))}
                {filtered.length === 0 && !loading && (
                    <div className="p-8 text-center text-gray-600 italic">
                        {filter ? `No packages matching "${filter}"` : 'No packages available'}
                    </div>
                )}
            </div>

            <div className="h-6 bg-black/40 text-[10px] text-gray-600 flex items-center px-4 border-t border-white/5">
                {installing ? `Installing ${installing}...` : uninstalling ? `Removing ${uninstalling}...` : `${filtered.length} packages shown · ${installedCount} installed`}
            </div>
        </div>
    );
};