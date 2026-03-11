import React, { useState } from 'react';
import * as Lucide from 'lucide-react';
import { useOS } from '../../store';
import { kernel } from '../../services/kernel';
import { Envelope } from '../../types';

export const Desktop: React.FC = () => {
    const { shortcuts, openWindow } = useOS();
    const [loadingShortcut, setLoadingShortcut] = useState<string | null>(null);

    const handleLaunchShortcut = (id: string, appletPath: string) => {
        if (loadingShortcut) return;
        setLoadingShortcut(id);

        // 1. Fetch raw TSX file from the Kernos File System
        const readReqId = Math.random().toString(36).substring(7);
        kernel.publish('vfs:read', { _request_id: readReqId, id: appletPath });

        const unsub = kernel.subscribe((env: Envelope) => {
            // 2. We got the raw TSX text
            if (env.topic === 'vfs:read:resp' && env.payload.id === appletPath) {

                const rawTSX = env.payload.content || '';
                if (!rawTSX) {
                    alert(`Shortcut Failed: Could not read ${appletPath}`);
                    setLoadingShortcut(null);
                    unsub();
                    return;
                }

                const appletId = appletPath.replace('.tsx', '');

                // 3. Command the kernel to compile the TSX into browser-JS on the fly
                kernel.publish('applet.compile', {
                    appletId,
                    code: rawTSX
                });
            }

            // 4. We got the compiled JS blob. Inject into OS.
            if (env.topic === 'applet.compile:success') {
                const { appletId, code } = env.payload;
                setLoadingShortcut(null);
                openWindow('applet', `Applet: ${appletId}`, { appletId, sourceCode: code });
                unsub();
            }

            // Compile failure
            if (env.topic === 'applet.compile:error') {
                setLoadingShortcut(null);
                alert(`Compilation Failed:\n\n${env.payload.error}`);
                unsub();
            }
        });

        // Cleanup timeout in case backend is dead
        setTimeout(() => {
            if (loadingShortcut === id) {
                setLoadingShortcut(null);
                unsub();
            }
        }, 5000);
    };

    return (
        <div className="absolute inset-0 z-0 p-6 flex flex-col gap-4 flex-wrap content-start">
            {shortcuts.map((shortcut) => {
                const Icon = (Lucide as any)[shortcut.icon] || Lucide.FileCode;
                const isLoading = loadingShortcut === shortcut.id;

                return (
                    <div
                        key={shortcut.id}
                        onDoubleClick={() => handleLaunchShortcut(shortcut.id, shortcut.appletPath)}
                        className={`w-24 group flex flex-col items-center gap-2 p-2 rounded hover:bg-white/10 cursor-pointer transition-colors ${isLoading ? 'opacity-50 pointer-events-none' : ''}`}
                        title={`Double-click to launch ${shortcut.name}`}
                    >
                        <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-indigo-500/20 to-cyan-500/20 shadow-[0_4px_20px_rgba(0,0,0,0.3)] border border-white/10 flex items-center justify-center relative group-hover:border-white/20 transition-all group-hover:scale-105">
                            {isLoading ? (
                                <Lucide.Loader className="text-white animate-spin" size={24} />
                            ) : (
                                <Icon className="text-cyan-400 drop-shadow-[0_0_8px_rgba(34,211,238,0.5)]" size={24} />
                            )}
                        </div>
                        <span className="text-white text-xs font-medium text-center drop-shadow-md px-1 select-none leading-tight">
                            {shortcut.name}
                        </span>
                    </div>
                );
            })}
        </div>
    );
};
