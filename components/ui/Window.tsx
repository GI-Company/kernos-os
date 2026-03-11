import React, { useState, useRef, useEffect } from 'react';
import { useOS } from '../../store';
import { WindowState, COLORS } from '../../types';
import { X, Minus, Square, Terminal, Monitor, FileCode, HardDrive, Cpu, Workflow, Package, Bot, Brain, FolderGit2, Activity, Settings } from 'lucide-react';

interface WindowProps {
  data: WindowState;
  children: React.ReactNode;
}

export const Window: React.FC<WindowProps> = ({ data, children }) => {
  const { closeWindow, focusWindow, moveWindow, resizeWindow, minimizeWindow, maximizeWindow } = useOS();
  const [isDragging, setIsDragging] = useState(false);
  const [snapZone, setSnapZone] = useState<'left' | 'right' | 'top' | null>(null);
  const dragOffset = useRef({ x: 0, y: 0 });
  const preSnapSize = useRef({ width: data.width, height: data.height, x: data.x, y: data.y });

  const SNAP_THRESHOLD = 20;

  const handleMouseDown = (e: React.MouseEvent) => {
    focusWindow(data.id);
    if (!data.isMaximized) {
      setIsDragging(true);
      preSnapSize.current = { width: data.width, height: data.height, x: data.x, y: data.y };
      dragOffset.current = {
        x: e.clientX - data.x,
        y: e.clientY - data.y
      };
    }
  };

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (isDragging) {
        const newX = e.clientX - dragOffset.current.x;
        const newY = e.clientY - dragOffset.current.y;
        moveWindow(data.id, newX, newY);

        // Detect snap zones
        const vw = window.innerWidth;
        if (e.clientX <= SNAP_THRESHOLD) {
          setSnapZone('left');
        } else if (e.clientX >= vw - SNAP_THRESHOLD) {
          setSnapZone('right');
        } else if (e.clientY <= SNAP_THRESHOLD) {
          setSnapZone('top');
        } else {
          setSnapZone(null);
        }
      }
    };

    const handleMouseUp = () => {
      if (isDragging && snapZone) {
        const vw = window.innerWidth;
        const vh = window.innerHeight - 48; // subtract taskbar height

        if (snapZone === 'left') {
          moveWindow(data.id, 0, 0);
          resizeWindow(data.id, vw / 2, vh);
        } else if (snapZone === 'right') {
          moveWindow(data.id, vw / 2, 0);
          resizeWindow(data.id, vw / 2, vh);
        } else if (snapZone === 'top') {
          maximizeWindow(data.id);
        }
      }
      setIsDragging(false);
      setSnapZone(null);
    };

    if (isDragging) {
      window.addEventListener('mousemove', handleMouseMove);
      window.addEventListener('mouseup', handleMouseUp);
    }
    return () => {
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('mouseup', handleMouseUp);
    };
  }, [isDragging, snapZone, data.id, moveWindow, resizeWindow, maximizeWindow]);

  if (data.isMinimized) return null;

  const getIcon = () => {
    switch (data.appId) {
      case 'terminal': return <Terminal size={14} className="text-cyan-400" />;
      case 'editor': return <FileCode size={14} className="text-purple-400" />;
      case 'monitor': return <Monitor size={14} className="text-orange-400" />;
      case 'packages': return <Package size={16} className="text-gray-400" />;
      case 'ai-chat': return <Brain size={16} className="text-pink-400" />;
      case 'agents': return <Bot size={16} className="text-blue-400" />;
      case 'semantic-vfs': return <FolderGit2 size={16} className="text-pink-400" />;
      case 'metrics': return <Activity size={16} className="text-cyan-400" />;
      case 'settings': return <Settings size={16} className="text-yellow-400" />;
      case 'multi-agents': return <Bot size={16} className="text-purple-400" />;
      default: return <Terminal size={16} className="text-gray-400" />;
    }
  };

  const borderClass = data.id === useOS.getState().activeWindowId
    ? 'border-white/20 shadow-[0_0_40px_rgba(0,0,0,0.5)]'
    : 'border-white/5 shadow-xl';

  const style = data.isMaximized
    ? { top: 0, left: 0, width: '100%', height: 'calc(100% - 48px)', transform: 'none' }
    : { top: data.y, left: data.x, width: data.width, height: data.height };

  return (
    <div
      style={{ ...style, zIndex: data.zIndex }}
      className={`absolute flex flex-col bg-[#0f0f13]/95 backdrop-blur-xl rounded-lg overflow-hidden border transition-shadow duration-200 ${borderClass}`}
      onMouseDown={() => focusWindow(data.id)}
    >
      {/* Title Bar */}
      <div
        onMouseDown={handleMouseDown}
        className="h-9 flex items-center justify-between px-3 bg-white/5 border-b border-white/5 select-none cursor-default active:cursor-grabbing"
      >
        <div className="flex items-center gap-3">
          {getIcon()}
          <span className="text-xs font-medium text-gray-300 tracking-wide">{data.title}</span>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={(e) => { e.stopPropagation(); minimizeWindow(data.id); }} className="p-1 hover:bg-white/10 rounded text-gray-500 hover:text-white">
            <Minus size={12} />
          </button>
          <button onClick={(e) => { e.stopPropagation(); maximizeWindow(data.id); }} className="p-1 hover:bg-white/10 rounded text-gray-500 hover:text-white">
            <Square size={10} />
          </button>
          <button onClick={(e) => { e.stopPropagation(); closeWindow(data.id); }} className="p-1 hover:bg-red-500/20 hover:text-red-400 rounded text-gray-500 transition-colors">
            <X size={12} />
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-hidden relative">
        {children}
      </div>

      {/* Footer / Status (Optional) */}
      <div className="h-5 bg-black/40 border-t border-white/5 flex items-center px-2">
        <div className="text-[10px] text-gray-600 font-mono">
          ID: {data.id} | MEM: {Math.floor(Math.random() * 128)}MB
        </div>
      </div>

      {/* Visual Snap Indicator */}
      {isDragging && snapZone && (
        <div 
          className="fixed pointer-events-none bg-cyan-500/20 border-2 border-cyan-400/50 rounded-lg shadow-[0_0_30px_rgba(0,240,255,0.2)] transition-all duration-200 z-[9999]"
          style={{
            top: 0,
            left: snapZone === 'right' ? '50%' : 0,
            width: snapZone === 'top' ? '100%' : '50%',
            height: snapZone === 'top' ? '100%' : '100%',
          }}
        />
      )}
    </div>
  );
};