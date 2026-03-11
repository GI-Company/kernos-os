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
import { kernel } from './services/kernel';

type BootPhase = 'boot' | 'login' | 'desktop';

const BOOT_LINES = [
  '[  0.000] KERNOS v1.0 — AI-Native Browser Operating System',
  '[  0.102] Initializing WebSocket Event Bus...',
  '[  0.204] Loading Zero-Trust Authentication Module...',
  '[  0.318] Mounting Virtual Filesystem (SQLite)...',
  '[  0.425] Starting Prediction Engine...',
  '[  0.531] Starting Task Engine (DAG + ReAct)...',
  '[  0.640] Starting Shadow Execution Pipeline...',
  '[  0.748] Loading Embedded Agent Configurations...',
  '[  0.856] Starting Cron Daemon (Metrics + Consolidation)...',
  '[  0.964] Starting Filesystem Watcher (fsnotify)...',
  '[  1.072] Starting P2P Gateway (WebRTC Signaling)...',
  '[  1.180] Binding to ws://localhost:8080/ws...',
  '[  1.300] System Ready. Awaiting authentication...',
];

const App: React.FC = () => {
  const { windows } = useOS();
  const [phase, setPhase] = useState<BootPhase>('boot');
  const [bootLines, setBootLines] = useState<string[]>([]);
  const [user, setUser] = useState<{ username: string; avatar_url: string; role: string } | null>(null);

  // Boot sequence animation
  useEffect(() => {
    let lineIndex = 0;
    const interval = setInterval(() => {
      if (lineIndex < BOOT_LINES.length) {
        setBootLines(prev => [...prev, BOOT_LINES[lineIndex]]);
        lineIndex++;
      } else {
        clearInterval(interval);
        // Boot complete → check local session
        setTimeout(() => {
          const guestSession = localStorage.getItem('kernos_guest_user');
          if (guestSession) {
            setUser(JSON.parse(guestSession));
            setPhase('desktop');
          } else {
            setPhase('login');
          }
        }, 600);
      }
    }, 120);
    return () => clearInterval(interval);
  }, []);

  const handleLogin = () => {
    // If backend is available, redirect to OAuth
    if (kernel.isLive) {
      window.location.href = '/auth/github';
    } else {
      // Dev mode — fallback to guest
      handleGuestAccess();
    }
  };

  const handleGuestAccess = () => {
    const guestId = Math.random().toString(36).substring(2, 6);
    const guestUser = {
      username: `Guest_${guestId}`,
      avatar_url: `https://api.dicebear.com/7.x/identicon/svg?seed=${guestId}`,
      role: 'guest'
    };
    localStorage.setItem('kernos_guest_user', JSON.stringify(guestUser));
    setUser(guestUser);
    setPhase('desktop');
  };

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

  // ─── BOOT SCREEN ───
  if (phase === 'boot') {
    return (
      <div className="w-screen h-screen bg-[#050505] flex flex-col items-center justify-center overflow-hidden">
        <div className="w-full max-w-2xl px-8">
          {/* Logo */}
          <div className="text-center mb-8">
            <h1 className="text-5xl font-black text-white/90 tracking-tighter">KERNOS</h1>
            <div className="h-0.5 w-20 bg-gradient-to-r from-transparent via-cyan-500 to-transparent mx-auto mt-3" />
          </div>
          {/* Boot log */}
          <div className="bg-black/60 border border-white/5 rounded-lg p-4 font-mono text-xs max-h-80 overflow-hidden">
            {bootLines.map((line, i) => (
              <div
                key={i}
                className={`leading-relaxed ${
                  line && line.includes('Ready') ? 'text-green-400' :
                  line && line.includes('Error') ? 'text-red-400' :
                  'text-cyan-500/70'
                }`}
                style={{ animation: 'fadeIn 0.15s ease-in' }}
              >
                {line || ''}
              </div>
            ))}
            {bootLines.length < BOOT_LINES.length && (
              <span className="text-white animate-pulse">▊</span>
            )}
          </div>
          {/* Progress bar */}
          <div className="mt-4 h-1 bg-white/5 rounded-full overflow-hidden">
            <div
              className="h-full bg-gradient-to-r from-cyan-500 to-purple-500 rounded-full transition-all duration-300 ease-out"
              style={{ width: `${(bootLines.length / BOOT_LINES.length) * 100}%` }}
            />
          </div>
        </div>
      </div>
    );
  }

  // ─── LOGIN SCREEN ───
  if (phase === 'login') {
    return (
      <div className="w-screen h-screen bg-[#050505] flex items-center justify-center overflow-hidden relative">
        {/* Background grid */}
        <div
          className="absolute inset-0 pointer-events-none opacity-10"
          style={{
            backgroundImage: `linear-gradient(to right, #1f1f1f 1px, transparent 1px), linear-gradient(to bottom, #1f1f1f 1px, transparent 1px)`,
            backgroundSize: '40px 40px'
          }}
        />
        {/* Glow effect */}
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-cyan-500/5 rounded-full blur-3xl pointer-events-none" />

        <div className="relative z-10 text-center">
          <h1 className="text-7xl font-black text-white/90 tracking-tighter mb-2">KERNOS</h1>
          <p className="text-white/20 font-mono tracking-[0.5em] text-sm mb-12">BROWSER NATIVE OS</p>

          <div className="space-y-3">
            <button
              onClick={handleLogin}
              className="w-72 inline-flex items-center justify-center gap-3 px-8 py-4 bg-white/[0.07] hover:bg-white/[0.12] border border-white/10 hover:border-white/20 rounded-xl text-white font-medium transition-all shadow-[0_0_30px_rgba(0,0,0,0.5)] hover:shadow-[0_0_40px_rgba(0,240,255,0.1)]"
            >
              <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor">
                <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
              </svg>
              Login with GitHub
            </button>

            <div>
              <button
                onClick={handleGuestAccess}
                className="text-gray-600 hover:text-gray-400 text-xs font-mono transition-colors"
              >
                Continue as Guest →
              </button>
            </div>
          </div>

          <p className="text-gray-700 text-[10px] mt-8 font-mono">Zero-Trust Authentication • OAuth 2.0 • RBAC</p>
        </div>
      </div>
    );
  }

  // ─── DESKTOP ───
  return (
    <div className="w-screen h-screen bg-[#050505] overflow-hidden relative selection:bg-cyan-500/30">
      {/* Dynamic Background Grid */}
      <div
        className="absolute inset-0 pointer-events-none opacity-20"
        style={{
          backgroundImage: `linear-gradient(to right, #1f1f1f 1px, transparent 1px), linear-gradient(to bottom, #1f1f1f 1px, transparent 1px)`,
          backgroundSize: '40px 40px'
        }}
      />

      {/* Desktop Area */}
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
      {windows.filter(win => win.desktopIndex === useOS.getState().currentDesktop).map(win => (
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