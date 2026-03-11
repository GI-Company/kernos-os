import React, { useState, useEffect } from 'react';
import { useOS } from '../../store';
import { Terminal, Monitor, FileCode, HardDrive, Cpu, Menu, Workflow, Package, Bot, Brain, FolderGit2, Sparkles, Activity, Settings, Users } from 'lucide-react';
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

export const Taskbar: React.FC = () => {
  const { windows, activeWindowId, focusWindow, minimizeWindow, openWindow } = useOS();

  return (
    <div className="h-12 w-full bg-[#0a0a0f]/80 backdrop-blur-md border-t border-white/5 flex items-center px-4 justify-between z-50 absolute bottom-0 select-none">
      <div className="flex items-center gap-4">
        <button className="p-2 rounded hover:bg-white/10 text-cyan-400 transition-colors">
          <Menu size={20} />
        </button>

        {/* Pinned Apps */}
        <div className="h-6 w-px bg-white/10 mx-2" />
        <button onClick={() => openWindow('terminal')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-white transition-colors" title="New Terminal">
          <Terminal size={18} />
        </button>
        <button onClick={() => openWindow('editor')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-white transition-colors" title="New Editor">
          <FileCode size={18} />
        </button>
        <button onClick={() => openWindow('files')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-green-400 transition-colors" title="Filesystem">
          <HardDrive size={18} />
        </button>
        <button onClick={() => openWindow('semantic-vfs', 'Semantic VFS')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-pink-400 transition-colors" title="Semantic VFS">
          <FolderGit2 size={18} />
        </button>
        <button onClick={() => openWindow('packages')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-white transition-colors" title="Package Manager">
          <Package size={18} />
        </button>
        <button onClick={() => openWindow('tasks')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-white transition-colors" title="Task Engine">
          <Workflow size={18} />
        </button>
        <button onClick={() => openWindow('monitor')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-white transition-colors" title="System Monitor">
          <Cpu size={18} />
        </button>
        <button onClick={() => openWindow('ai-chat', 'AI Chat')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-purple-400 transition-colors" title="AI Chat">
          <Brain size={18} />
        </button>
        <button onClick={() => openWindow('agents', 'Agent Monitor')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-green-400 transition-colors" title="Agent Monitor">
          <Bot size={18} />
        </button>
        <button
          onClick={() => {
            alert('Triggering Nightly Synaptic Consolidation. Check terminal logs.');
            kernel.publish('sys.consolidate', {});
          }}
          className="p-2 rounded hover:bg-pink-500/20 text-pink-400/50 hover:text-pink-400 transition-colors"
          title="Consolidate RLHF Memory"
        >
          <Sparkles size={18} />
        </button>
        <button onClick={() => openWindow('metrics', 'System Metrics')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-cyan-400 transition-colors" title="System Metrics">
          <Activity size={18} />
        </button>
        <button onClick={() => openWindow('settings', 'Settings')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-yellow-400 transition-colors" title="Settings">
          <Settings size={18} />
        </button>
        <button onClick={() => openWindow('multi-agents', 'Multi-Agent Workspace')} className="p-2 rounded hover:bg-white/10 text-gray-400 hover:text-purple-400 transition-colors" title="Multi-Agent Workspace">
          <Users size={18} />
        </button>

        <div className="h-6 w-px bg-white/10 mx-2" />

        {/* Running Windows */}
        <div className="flex gap-2">
          {windows.map((win) => (
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
              {win.appId === 'terminal' && <Terminal size={14} />}
              {win.appId === 'editor' && <FileCode size={14} />}
              {win.appId === 'monitor' && <Monitor size={14} />}
              {win.appId === 'files' && <HardDrive size={14} />}
              {win.appId === 'tasks' && <Workflow size={14} />}
              {win.appId === 'packages' && <Package size={14} />}
              {win.appId === 'ai-chat' && <Brain size={14} />}
              {win.appId === 'agents' && <Bot size={14} />}
              {win.appId === 'semantic-vfs' && <FolderGit2 size={14} />}
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