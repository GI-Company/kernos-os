import React, { useState, useEffect } from 'react';
import { kernel } from '../services/kernel';
import { useOS } from '../store';
import { Envelope, FileNode } from '../types';
import { Folder, FileText, ChevronRight, Home, RefreshCw, FilePlus, FolderPlus, Trash2, Edit2, Cloud } from 'lucide-react';

export const FileSystemApp: React.FC = () => {
  const [files, setFiles] = useState<FileNode[]>([]);
  const [currentPath, setCurrentPath] = useState('home');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const { openWindow } = useOS();

  const refresh = () => {
      setIsLoading(true);
      const reqId = Math.random().toString();
      kernel.publish('vfs:list', { _request_id: reqId, path: currentPath });
  };

  useEffect(() => {
    refresh();
    const unsub = kernel.subscribe((env: Envelope) => {
        if (env.topic === 'vfs:list:resp') {
            // Only update if the response corresponds to our current path or we are waiting
            // In a real app we'd match _request_id
            if (env.payload.path === currentPath) {
                setFiles(env.payload.files);
                setIsLoading(false);
            }
        }
        
        // Listen for VFS changes via the watch topic
        if (env.topic === 'vfs:watch') {
            const { parentId } = env.payload;
            // If the change occurred in the current directory (or affects it), refresh.
            if (parentId === currentPath) {
                refresh();
            }
        }
    });
    return unsub;
  }, [currentPath]);

  const handleDoubleClick = (file: FileNode) => {
      if (file.type === 'directory') {
          setCurrentPath(file.id);
      } else {
          openWindow('editor', file.name, { fileId: file.id, fileName: file.name });
      }
  };

  const handleCreate = (type: 'file' | 'directory') => {
      const name = prompt(`Enter ${type} name:`);
      if (name) {
          const reqId = Math.random().toString();
          kernel.publish('vfs:create', { 
              _request_id: reqId, 
              parentId: currentPath, 
              name, 
              type 
          });
      }
  };

  const handleRename = () => {
      if (!selectedId) return;
      const file = files.find(f => f.id === selectedId);
      if (!file) return;

      const newName = prompt(`Rename ${file.name} to:`, file.name);
      if (newName && newName !== file.name) {
          const reqId = Math.random().toString();
          kernel.publish('vfs:rename', { _request_id: reqId, id: selectedId, newName });
      }
  };

  const handleDelete = () => {
      if (!selectedId) return;
      if (confirm('Delete selected item?')) {
        const reqId = Math.random().toString();
        kernel.publish('vfs:delete', { _request_id: reqId, id: selectedId });
        setSelectedId(null);
      }
  };

  return (
    <div className="h-full bg-[#18181b] text-gray-200 flex flex-col">
        {/* Toolbar */}
        <div className="h-10 border-b border-white/5 flex items-center px-4 gap-2 text-sm bg-white/5">
            <button onClick={() => setCurrentPath('root')} className="p-1 hover:text-white text-gray-400"><Home size={14} /></button>
            <ChevronRight size={14} className="text-gray-600" />
            <span className="font-mono text-cyan-500">{currentPath}</span>
            <div className="flex-1" />
            <button onClick={() => handleCreate('file')} className="p-1.5 hover:bg-white/10 rounded text-gray-400" title="New File"><FilePlus size={14}/></button>
            <button onClick={() => handleCreate('directory')} className="p-1.5 hover:bg-white/10 rounded text-gray-400" title="New Folder"><FolderPlus size={14}/></button>
            <button onClick={handleRename} className="p-1.5 hover:bg-white/10 rounded text-gray-400" title="Rename"><Edit2 size={14}/></button>
            <button onClick={handleDelete} className="p-1.5 hover:bg-red-500/20 text-gray-400 hover:text-red-400 rounded" title="Delete"><Trash2 size={14}/></button>
            <div className="w-px h-4 bg-white/10 mx-2" />
            <button onClick={refresh} className={`p-1.5 hover:bg-white/10 rounded text-gray-400 ${isLoading ? 'animate-spin' : ''}`}><RefreshCw size={14}/></button>
        </div>

        {/* Grid */}
        <div className="p-4 grid grid-cols-4 gap-4 overflow-y-auto flex-1">
            {currentPath !== 'root' && (
                <button 
                    onClick={() => {
                        // Very naive parent navigation for mock
                        setCurrentPath('root'); 
                    }}
                    className="flex flex-col items-center justify-center p-4 rounded border border-transparent hover:bg-white/5 text-gray-500"
                >
                    <Folder size={32} className="text-gray-600" />
                    <span className="mt-2 text-xs">..</span>
                </button>
            )}
            {files.map(file => {
                const isMount = !!file.mountSource;
                return (
                    <button 
                        key={file.id}
                        onClick={() => setSelectedId(file.id)}
                        onDoubleClick={() => handleDoubleClick(file)}
                        className={`
                            flex flex-col items-center justify-center p-4 rounded border transition-colors group relative
                            ${selectedId === file.id ? 'bg-cyan-500/10 border-cyan-500/50 text-cyan-100' : 'bg-white/5 border-transparent hover:bg-white/10 text-gray-400'}
                        `}
                    >
                        {file.type === 'directory' 
                            ? (isMount ? <Cloud size={32} className={selectedId === file.id ? 'text-cyan-400' : 'text-blue-400 group-hover:text-blue-300'} /> : <Folder size={32} className={selectedId === file.id ? 'text-cyan-400' : 'text-yellow-500 group-hover:text-yellow-400'} />) 
                            : <FileText size={32} className={selectedId === file.id ? 'text-cyan-400' : 'text-gray-500 group-hover:text-gray-300'} />
                        }
                        <span className="mt-2 text-xs truncate w-full text-center select-none flex items-center justify-center gap-1">
                            {file.name}
                            {isMount && <span className="w-1.5 h-1.5 rounded-full bg-green-500" title="Connected"></span>}
                        </span>
                    </button>
                );
            })}
        </div>
        <div className="h-6 bg-[#0f0f13] border-t border-white/5 px-2 text-[10px] text-gray-600 flex items-center justify-between">
            <span>{files.length} items {isLoading && '(Loading...)'}</span>
            <span>{selectedId ? `Selected: ${selectedId}` : 'Ready'}</span>
        </div>
    </div>
  );
};