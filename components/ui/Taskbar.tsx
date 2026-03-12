import React, { useState, useEffect, useRef } from 'react';
import { useOS } from '../../store';
import { Terminal, Monitor, FileCode, HardDrive, Cpu, Menu, Workflow, Package, Bot, Brain, FolderGit2, Sparkles, Activity, Settings, Users, Clock as ClockIcon, Globe, X } from 'lucide-react';
import { kernel } from '../../services/kernel';

const Clock = () => {
  const [time, setTime] = useState(new Date());
  useEffect(() => {
    const timer = setInterval(() => setTime(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);
  return (
    <div className="text-xs font-mono text-gray-400">
      {time.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
    </div>
  );
};

/* Tooltip wrapper — shows label on hover */
const TipButton: React.FC<{ label: string; onClick: () => void; className?: string; children: React.ReactNode }> = ({ label, onClick, className, children }) => (
  <div className="relative group">
    <button onClick={onClick} className={className} aria-label={label}>
      {children}
    </button>
    <span className="pointer-events-none absolute -top-8 left-1/2 -translate-x-1/2 whitespace-nowrap rounded bg-black/90 border border-white/10 px-2 py-0.5 text-[10px] text-gray-300 font-mono opacity-0 group-hover:opacity-100 transition-opacity duration-150 z-[999]">
      {label}
    </span>
  </div>
);

export const Taskbar: React.FC = () => {
  const { windows, activeWindowId, focusWindow, minimizeWindow, openWindow } = useOS();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close menu on outside click
  useEffect(() => {
    if (!menuOpen) return;
    const handler = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [menuOpen]);

  const allApps = [
    { id: 'terminal',     label: 'Terminal',            icon: <Terminal size={16} /> },
    { id: 'editor',       label: 'Code Editor',         icon: <FileCode size={16} /> },
    { id: 'files',        label: 'File System',         icon: <HardDrive size={16} /> },
    { id: 'semantic-vfs', label: 'Semantic VFS',        icon: <FolderGit2 size={16} /> },
    { id: 'packages',     label: 'Package Manager',     icon: <Package size={16} /> },
    { id: 'tasks',        label: 'Task Engine',         icon: <Workflow size={16} /> },
    { id: 'monitor',      label: 'System Monitor',      icon: <Cpu size={16} /> },
    { id: 'ai-chat',      label: 'AI Chat',             icon: <Brain size={16} /> },
    { id: 'agents',       label: 'Agent Monitor',       icon: <Bot size={16} /> },
    { id: 'metrics',      label: 'System Metrics',      icon: <Activity size={16} /> },
    { id: 'settings',     label: 'Settings',            icon: <Settings size={16} /> },
    { id: 'multi-agents', label: 'Multi-Agent Workspace', icon: <Users size={16} /> },
    { id: 'timeline',     label: 'Timeline',            icon: <ClockIcon size={16} /> },
    { id: 'p2p',          label: 'P2P Portal',          icon: <Globe size={16} /> },
  ];

  /* Pinned icons on the taskbar (subset of all apps) */
  const pinned = [
    { id: 'terminal',     label: 'Terminal',            icon: <Terminal size={18} />,  color: 'hover:text-white' },
    { id: 'editor',       label: 'Code Editor',         icon: <FileCode size={18} />, color: 'hover:text-white' },
    { id: 'files',        label: 'File System',         icon: <HardDrive size={18} />, color: 'hover:text-green-400' },
    { id: 'semantic-vfs', label: 'Semantic VFS',        icon: <FolderGit2 size={18} />, color: 'hover:text-pink-400' },
    { id: 'packages',     label: 'Package Manager',     icon: <Package size={18} />,  color: 'hover:text-white' },
    { id: 'tasks',        label: 'Task Engine',         icon: <Workflow size={18} />,  color: 'hover:text-white' },
    { id: 'monitor',      label: 'System Monitor',      icon: <Cpu size={18} />,      color: 'hover:text-white' },
    { id: 'ai-chat',      label: 'AI Chat',             icon: <Brain size={18} />,    color: 'hover:text-purple-400' },
    { id: 'agents',       label: 'Agent Monitor',       icon: <Bot size={18} />,      color: 'hover:text-green-400' },
    { id: 'metrics',      label: 'System Metrics',      icon: <Activity size={18} />, color: 'hover:text-cyan-400' },
    { id: 'settings',     label: 'Settings',            icon: <Settings size={18} />, color: 'hover:text-yellow-400' },
    { id: 'multi-agents', label: 'Multi-Agent Workspace', icon: <Users size={18} />,  color: 'hover:text-purple-400' },
    { id: 'timeline',     label: 'Timeline',            icon: <ClockIcon size={18} />, color: 'hover:text-orange-400' },
    { id: 'p2p',          label: 'P2P Portal',          icon: <Globe size={18} />,    color: 'hover:text-blue-400' },
  ];

  const iconForAppId = (appId: string) => {
    switch (appId) {
      case 'terminal':     return <Terminal size={14} />;
      case 'editor':       return <FileCode size={14} />;
      case 'monitor':      return <Monitor size={14} />;
      case 'files':        return <HardDrive size={14} />;
      case 'tasks':        return <Workflow size={14} />;
      case 'packages':     return <Package size={14} />;
      case 'ai-chat':      return <Brain size={14} />;
      case 'agents':       return <Bot size={14} />;
      case 'semantic-vfs': return <FolderGit2 size={14} />;
      case 'metrics':      return <Activity size={14} />;
      case 'settings':     return <Settings size={14} />;
      case 'multi-agents': return <Users size={14} />;
      case 'timeline':     return <ClockIcon size={14} />;
      case 'p2p':          return <Globe size={14} />;
      default:             return null;
    }
  };

  return (
    <div className="h-12 w-full bg-[#0a0a0f]/80 backdrop-blur-md border-t border-white/5 flex items-center px-4 justify-between z-50 absolute bottom-0 select-none">
      <div className="flex items-center gap-4">
        {/* Hamburger / App Launcher */}
        <div className="relative" ref={menuRef}>
          <TipButton label="All Apps" onClick={() => setMenuOpen(o => !o)} className="p-2 rounded hover:bg-white/10 text-cyan-400 transition-colors">
            {menuOpen ? <X size={20} /> : <Menu size={20} />}
          </TipButton>

          {menuOpen && (
            <div className="absolute bottom-12 left-0 w-56 bg-[#0d0d14]/95 backdrop-blur-xl border border-white/10 rounded-lg shadow-2xl shadow-black/60 p-2 flex flex-col gap-0.5 z-[999]">
              <div className="px-3 py-1.5 text-[10px] text-gray-500 font-mono uppercase tracking-widest">All Applications</div>
              {allApps.map(app => (
                <button
                  key={app.id}
                  onClick={() => { openWindow(app.id as any, app.label); setMenuOpen(false); }}
                  className="flex items-center gap-3 px-3 py-2 rounded-md text-sm text-gray-300 hover:bg-white/10 hover:text-white transition-colors"
                >
                  {app.icon}
                  <span className="font-mono text-xs">{app.label}</span>
                </button>
              ))}
              <div className="border-t border-white/5 mt-1 pt-1">
                <button
                  onClick={() => {
                    kernel.publish('sys.consolidate', {});
                    setMenuOpen(false);
                  }}
                  className="flex items-center gap-3 px-3 py-2 rounded-md text-sm text-pink-400/70 hover:bg-pink-500/10 hover:text-pink-400 transition-colors w-full"
                >
                  <Sparkles size={16} />
                  <span className="font-mono text-xs">Consolidate Memory</span>
                </button>
              </div>
            </div>
          )}
        </div>

        {/* Pinned Apps */}
        <div className="h-6 w-px bg-white/10 mx-2" />
        {pinned.map(app => (
          <TipButton
            key={app.id}
            label={app.label}
            onClick={() => openWindow(app.id as any, app.label)}
            className={`p-2 rounded hover:bg-white/10 text-gray-400 ${app.color} transition-colors`}
          >
            {app.icon}
          </TipButton>
        ))}

        <div className="h-6 w-px bg-white/10 mx-2" />

        {/* Virtual Desktops */}
        <div className="flex gap-1 bg-white/5 rounded p-1">
          {[0, 1, 2, 3].map(idx => (
            <button
              key={idx}
              onClick={() => useOS.getState().switchDesktop(idx)}
              className={`w-6 h-6 rounded flex items-center justify-center text-xs font-mono transition-colors ${
                useOS.getState().currentDesktop === idx
                  ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30'
                  : 'text-gray-500 hover:bg-white/10 hover:text-white'
              }`}
            >
              {idx + 1}
            </button>
          ))}
        </div>

        <div className="h-6 w-px bg-white/10 mx-2" />

        {/* Running Windows (Current Desktop Only) */}
        <div className="flex gap-2">
          {windows.filter(win => win.desktopIndex === useOS.getState().currentDesktop).map((win) => (
            <button
              key={win.id}
              onClick={() => win.id === activeWindowId && !win.isMinimized ? minimizeWindow(win.id) : focusWindow(win.id)}
              className={`
                px-3 py-1.5 rounded flex items-center gap-2 text-sm border transition-all
                ${win.id === activeWindowId && !win.isMinimized
                  ? 'bg-white/10 border-white/20 text-white shadow-[0_0_10px_rgba(0,240,255,0.1)]'
                  : 'bg-transparent border-transparent text-gray-500 hover:bg-white/5 hover:text-gray-300'}
              `}
            >
              {iconForAppId(win.appId)}
              <span className="truncate max-w-[100px]">{win.title}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2 px-2 py-1 rounded bg-white/5 border border-white/5">
          <div className="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
          <span className="text-[10px] text-gray-400 font-mono tracking-wider">ONLINE</span>
        </div>
        <Clock />
      </div>
    </div>
  );
};