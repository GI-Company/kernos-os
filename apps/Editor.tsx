import React, { useState, useEffect } from 'react';
import { kernel } from '../services/kernel';
import { Envelope } from '../types';
import { useOS } from '../store';
import { Sparkles, Save, Play, Check, AlertCircle, LayoutTemplate } from 'lucide-react';

interface EditorProps {
  fileId?: string;
  fileName?: string;
}

export const EditorApp: React.FC<EditorProps> = (props) => {
  const [content, setContent] = useState('// Loading...');
  const [isDirty, setIsDirty] = useState(false);
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved'>('idle');
  const [aiStatus, setAiStatus] = useState<'idle' | 'thinking' | 'streaming'>('idle');
  const [aiMessage, setAiMessage] = useState('');
  const [isCompiling, setIsCompiling] = useState(false);
  const { openWindow } = useOS();

  // Debounce timer for predictive telemetry
  const typingTimeoutRef = React.useRef<NodeJS.Timeout>();

  // Load file content if fileId exists
  useEffect(() => {
    if (props.fileId) {
      const reqId = Math.random().toString();
      kernel.publish('vfs:read', { _request_id: reqId, id: props.fileId });

      const unsub = kernel.subscribe((env: Envelope) => {
        if (env.topic === 'vfs:read:resp' && env.payload.id === props.fileId) {
          setContent(env.payload.content || '');
        }
        if (env.topic === 'vfs:write:ack' && env.payload.id === props.fileId) {
          setSaveStatus('saved');
          setTimeout(() => setSaveStatus('idle'), 2000);
        }
      });
      return unsub;
    } else {
      setContent('// Untitled Buffer\n\nfunction main() {\n  console.log("Hello Kernos");\n}');
    }
  }, [props.fileId]);

  const handleAiAsk = () => {
    setAiStatus('thinking');
    setAiMessage('');
    const reqId = Math.random().toString(36).substring(7);
    kernel.publish('ai.chat', { _request_id: reqId, prompt: `Context:\n${content}\n\nExplain this code.` });
  };

  const handleSave = () => {
    if (props.fileId) {
      setSaveStatus('saving');
      const reqId = Math.random().toString();
      kernel.publish('vfs:write', { _request_id: reqId, id: props.fileId, content });
      setIsDirty(false);
    } else {
      alert("Save as not implemented in mock. Open a file from filesystem first.");
    }
  };

  useEffect(() => {
    const unsub = kernel.subscribe((env: Envelope) => {
      if (env.topic === 'ai.stream') {
        setAiStatus('streaming');
        setAiMessage(prev => prev + env.payload.chunk);
      }
      if (env.topic === 'ai.done') {
        setAiStatus('idle');
      }

      // Handle Applet Compilation
      if (env.topic === 'applet.compile:success') {
        setIsCompiling(false);
        const { appletId, code } = env.payload;
        // Mount the dynamic applet into the window manager
        openWindow('applet', `Applet: ${appletId}`, { appletId, sourceCode: code });
      }
      if (env.topic === 'applet.compile:error') {
        setIsCompiling(false);
        alert(`Compilation Failed:\n\n${env.payload.error}`);
      }
    });
    return unsub;
  }, []);

  const handleLaunchApplet = () => {
    if (!content.includes('export default') && !content.includes('return ')) {
      alert("Applets must export a default React component.");
      return;
    }

    setIsCompiling(true);
    const appletId = props.fileName ? props.fileName.replace('.tsx', '') : 'UntitledApplet';

    kernel.publish('applet.compile', {
      appletId,
      code: content
    });
  };

  return (
    <div className="h-full flex flex-col bg-[#1e1e1e] text-white">
      {/* Editor Toolbar */}
      <div className="h-10 bg-[#252526] border-b border-black flex items-center px-4 gap-2 justify-between">
        <div className="flex items-center gap-2">
          <button className="flex items-center gap-2 px-3 py-1 bg-cyan-600/20 hover:bg-cyan-600/30 text-cyan-400 text-xs rounded transition-colors" onClick={handleAiAsk}>
            <Sparkles size={12} />
            <span>AI Assist</span>
          </button>
          <div className="w-px h-4 bg-white/10 mx-2" />
          <button onClick={handleSave} className="p-1.5 hover:bg-white/10 rounded text-gray-400 flex items-center gap-1" title="Save">
            {saveStatus === 'saved' ? <Check size={14} className="text-green-400" /> : <Save size={14} className={isDirty ? 'text-yellow-400' : ''} />}
          </button>

          <div className="w-px h-4 bg-white/10 mx-2" />

          <button
            className={`flex items-center gap-2 px-3 py-1 ${isCompiling ? 'bg-orange-500/20 text-orange-400' : 'bg-green-600/20 hover:bg-green-600/30 text-green-400'} text-xs rounded transition-colors`}
            onClick={handleLaunchApplet}
            disabled={isCompiling}
            title="Compile & Launch as Browser Native Applet"
          >
            <LayoutTemplate size={12} className={isCompiling ? 'animate-pulse' : ''} />
            <span>{isCompiling ? 'Compiling...' : 'Launch Applet'}</span>
          </button>
        </div>
        <div className="text-xs text-gray-500 font-mono">
          {props.fileName || 'Untitled'} {isDirty && '•'}
        </div>
      </div>

      <div className="flex-1 flex overflow-hidden">
        {/* Line Numbers */}
        <div className="w-10 bg-[#1e1e1e] border-r border-white/5 flex flex-col items-end pr-2 pt-4 text-gray-600 font-mono text-sm select-none">
          {content.split('\n').map((_, i) => <div key={i}>{i + 1}</div>)}
        </div>

        {/* Text Area */}
        <textarea
          className="flex-1 bg-[#1e1e1e] text-gray-300 font-mono text-sm p-4 outline-none border-none resize-none"
          value={content}
          onChange={(e) => {
            const newContent = e.target.value;
            setContent(newContent);
            setIsDirty(true);

            // Debounced telemetry for Subconscious Execution Pipeline
            if (typingTimeoutRef.current) clearTimeout(typingTimeoutRef.current);
            typingTimeoutRef.current = setTimeout(() => {
              kernel.publish('editor.typing', {
                filename: props.fileName || 'Untitled',
                snippet: newContent.substring(newContent.length - 1000) // send last 1000 chars context
              });

              if (kernel.isLive && (kernel as any).socket) {
                (kernel as any).socket.send(JSON.stringify({
                  topic: 'editor.typing',
                  from: kernel.getClientId(),
                  payload: {
                    filename: props.fileName || 'Untitled',
                    snippet: newContent.substring(Math.max(0, newContent.length - 1000))
                  },
                  time: new Date().toISOString()
                }));
              }
            }, 1500); // 1.5s debounce
          }}
          spellCheck={false}
        />

        {/* AI Panel */}
        {(aiStatus !== 'idle' || aiMessage) && (
          <div className="w-64 bg-[#252526] border-l border-black p-4 flex flex-col">
            <div className="text-xs font-bold text-gray-400 mb-2 flex items-center gap-2">
              <Sparkles size={10} className="text-purple-400" />
              AI ASSISTANT
            </div>
            <div className="flex-1 text-sm text-gray-300 font-sans leading-relaxed whitespace-pre-wrap overflow-y-auto">
              {aiMessage}
              {aiStatus === 'thinking' && <span className="animate-pulse">_</span>}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};