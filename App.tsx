import React, { useEffect, useState } from 'react';
import { useOS } from './store';
import { Taskbar } from './components/ui/Taskbar';
import { Window } from './components/ui/Window';
import { TerminalApp } from './apps/Terminal';
import { EditorApp } from './apps/Editor';
import { CDEApp } from './apps/CDE';
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
import { LoginScreen } from './components/LoginScreen';
import { BIOSSetup } from './components/BIOSSetup';
import { MultiAgentWorkspace } from './apps/MultiAgentWorkspace';
import { TimelineSlider } from './apps/TimelineSlider';
import { CinematicBoot } from './components/ui/CinematicBoot';
import { ContextMenuProvider } from './components/ui/ContextMenu';
import { kernel } from './services/kernel';
import { AnimatePresence } from 'framer-motion';

type BootPhase = 'boot' | 'bios' | 'login' | 'desktop';

const App: React.FC = () => {
  const { windows } = useOS();
  const [phase, setPhase] = useState<BootPhase>('boot');
  const [bootLines, setBootLines] = useState<string[]>([]);
  const [user, setUser] = useState<{ username: string; avatar_url: string; role: string } | null>(null);
  const [biosRequested, setBiosRequested] = useState(false);

  // Listen for right-click during boot to enter BIOS
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (e.button === 2 && phase === 'boot') {
        e.preventDefault();
        setBiosRequested(true);
      }
    };
    const preventMenu = (e: MouseEvent) => {
      if (phase === 'boot') e.preventDefault();
    };
    window.addEventListener('mousedown', handler);
    window.addEventListener('contextmenu', preventMenu);
    return () => {
      window.removeEventListener('mousedown', handler);
      window.removeEventListener('contextmenu', preventMenu);
    };
  }, [phase]);

  const handleBootComplete = () => {
    if (biosRequested) {
      setPhase('bios');
      return;
    }
    const savedUser = localStorage.getItem('kernos_user');
    const guestSession = localStorage.getItem('kernos_guest_user');
    if (savedUser) {
      setUser(JSON.parse(savedUser));
      setPhase('desktop');
    } else if (guestSession) {
      setUser(JSON.parse(guestSession));
      setPhase('desktop');
    } else {
      setPhase('login');
    }
  };

  const handleLogin = (loginUser: any) => {
    const u = {
      username: loginUser.username || loginUser.display_name,
      avatar_url: loginUser.avatar_url || '',
      role: loginUser.role || 'user'
    };
    localStorage.setItem('kernos_user', JSON.stringify(u));
    setUser(u);
    setPhase('desktop');
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
      case 'cde': return <CDEApp />;
      case 'timeline': return <TimelineSlider />;
      default: return <div className="p-4 text-red-500">App not found</div>;
    }
  };

  // ─── BOOT SCREEN ───
  if (phase === 'boot') {
    return (
      <AnimatePresence>
        <CinematicBoot onComplete={handleBootComplete} />
      </AnimatePresence>
    );
  }

  // ─── BIOS SETUP ───
  if (phase === 'bios') {
    return <BIOSSetup onExit={() => setPhase('login')} />;
  }

  // ─── LOGIN SCREEN ───
  if (phase === 'login') {
    return <LoginScreen onLogin={handleLogin} onGuestAccess={handleGuestAccess} />;
  }

  // ─── DESKTOP ───
  return (
    <ContextMenuProvider>
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
            <h1 className="text-9xl font-black text-white/15 tracking-tighter">KERNOS</h1>
            <p className="text-white/25 font-mono mt-4 tracking-[1em]">BROWSER NATIVE OS</p>
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
    </ContextMenuProvider>
  );
};

export default App;