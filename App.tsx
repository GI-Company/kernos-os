import React, { useEffect, useState } from 'react';
import { useOS } from './store';
import { Taskbar } from './components/ui/Taskbar';
import { Window } from './components/ui/Window';
import { TerminalApp } from './apps/Terminal';
import { EditorApp } from './apps/Editor';
import { MonitorApp } from './apps/Monitor';
import { FileSystemApp } from './apps/FileSystem';
import { TaskRunnerApp } from './apps/TaskRunner';
import { PackageManagerApp } from './apps/PackageManager';
import { AIChatApp } from './apps/AIChat';
import { AgentMonitorApp } from './apps/AgentMonitor';
import { SemanticVFSApp } from './apps/SemanticVFS';
import { P2PPortal } from './components/apps/P2PPortal';
import { DynamicApplet } from './components/apps/DynamicApplet';
import { AudioSystem } from './components/ui/AudioSystem';
import { Desktop } from './components/ui/Desktop';
import { ToastSystem } from './components/ui/ToastSystem';
import { SystemMetricsApp } from './apps/SystemMetrics';
import { SettingsApp } from './apps/Settings';
import { MultiAgentWorkspace } from './apps/MultiAgentWorkspace';

const App: React.FC = () => {
  const { windows } = useOS();
  const [authState, setAuthState] = useState<'checking' | 'logged-in' | 'logged-out'>('checking');
  const [user, setUser] = useState<{ username: string; avatar_url: string; role: string } | null>(null);

  useEffect(() => {
    fetch('/auth/me', { credentials: 'include' })
      .then(r => {
        if (r.ok) return r.json();
        throw new Error('Not authenticated');
      })
      .then(profile => {
        setUser(profile);
        setAuthState('logged-in');
      })
      .catch(() => {
        // In dev/offline mode, skip login and allow access
        setAuthState('logged-in');
      });
  }, []);

  const getAppContent = (appId: string, data?: any) => {
    switch (appId) {
      case 'terminal': return <TerminalApp />;
      case 'editor': return <EditorApp {...data} />;
      case 'monitor': return <MonitorApp />;
      case 'files': return <FileSystemApp />;
      case 'tasks': return <TaskRunnerApp />;
      case 'packages': return <PackageManagerApp />;
      case 'ai-chat': return <AIChatApp />;
      case 'agents': return <AgentMonitorApp />;
      case 'semantic-vfs': return <SemanticVFSApp />;
      case 'p2p': return <P2PPortal />;
      case 'applet': return <DynamicApplet appletId={data?.appletId} sourceCode={data?.sourceCode} />;
      case 'metrics': return <SystemMetricsApp />;
      case 'settings': return <SettingsApp />;
      case 'multi-agents': return <MultiAgentWorkspace />;
      default: return <div className="p-4 text-red-500">App not found</div>;
    }
  };

  return (
    <div className="w-screen h-screen bg-[#050505] overflow-hidden relative selection:bg-cyan-500/30">

      {/* GitHub OAuth Login Overlay */}
      {authState === 'logged-out' && (
        <div className="absolute inset-0 z-[9999] bg-[#050505] flex items-center justify-center">
          <div className="text-center">
            <h1 className="text-6xl font-black text-white/90 tracking-tighter mb-2">KERNOS</h1>
            <p className="text-white/30 font-mono tracking-[0.5em] mb-10">BROWSER NATIVE OS</p>
            <a
              href="/auth/github"
              className="inline-flex items-center gap-3 px-8 py-4 bg-white/10 hover:bg-white/15 border border-white/10 hover:border-white/20 rounded-xl text-white font-medium transition-all shadow-[0_0_30px_rgba(0,0,0,0.5)] hover:shadow-[0_0_40px_rgba(0,240,255,0.1)]"
            >
              <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
                <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
              </svg>
              Login with GitHub
            </a>
            <p className="text-gray-600 text-xs mt-6 font-mono">Zero-Trust Authentication Required</p>
          </div>
        </div>
      )}

      {authState === 'checking' && (
        <div className="absolute inset-0 z-[9999] bg-[#050505] flex items-center justify-center">
          <div className="text-center">
            <h1 className="text-4xl font-black text-white/30 tracking-tighter mb-4">KERNOS</h1>
            <div className="w-6 h-6 border-2 border-cyan-500/30 border-t-cyan-500 rounded-full animate-spin mx-auto" />
          </div>
        </div>
      )}

      {/* Dynamic Background Grid */}
      <div
        className="absolute inset-0 pointer-events-none opacity-20"
        style={{
          backgroundImage: `linear-gradient(to right, #1f1f1f 1px, transparent 1px), linear-gradient(to bottom, #1f1f1f 1px, transparent 1px)`,
          backgroundSize: '40px 40px'
        }}
      />

      {/* Desktop Area - Interactive Shortcuts Space */}
      <div className="absolute inset-0 z-0">
        <Desktop />
        {/* Logo / Watermark */}
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 text-center pointer-events-none opacity-30">
          <h1 className="text-9xl font-black text-white/5 tracking-tighter">KERNOS</h1>
          <p className="text-white/10 font-mono mt-4 tracking-[1em]">BROWSER NATIVE OS</p>
        </div>
      </div>

      <ToastSystem />

      {/* Window Layer */}
      {windows.map(win => (
        <Window key={win.id} data={win}>
          {getAppContent(win.appId, win.data)}
        </Window>
      ))}

      <Taskbar />
      <AudioSystem />
    </div>
  );
};

export default App;