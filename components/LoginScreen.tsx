import React, { useState, useEffect } from 'react';
import { kernel } from '../services/kernel';
import { Envelope } from '../types';
import { User, Lock, Plus, LogIn, Loader2, Eye, EyeOff, Shield } from 'lucide-react';

// ═══════════════════════════════════════════════════════════════
//  Kernos Login Screen — macOS-style User Account Login
// ═══════════════════════════════════════════════════════════════

interface UserInfo {
  id: number;
  username: string;
  display_name: string;
  avatar_url: string;
  role: string;
  last_login: string;
}

interface LoginScreenProps {
  onLogin: (user: UserInfo) => void;
  onGuestAccess: () => void;
}

export const LoginScreen: React.FC<LoginScreenProps> = ({ onLogin, onGuestAccess }) => {
  const [users, setUsers] = useState<UserInfo[]>([]);
  const [hasUsers, setHasUsers] = useState<boolean | null>(null);
  const [selectedUser, setSelectedUser] = useState<UserInfo | null>(null);
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  // Signup fields
  const [isSignup, setIsSignup] = useState(false);
  const [newUsername, setNewUsername] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [newDisplayName, setNewDisplayName] = useState('');

  // Fetch users on mount
  useEffect(() => {
    kernel.publish('user.list', {});

    const unsub = kernel.subscribe((env: Envelope) => {
      if (env.topic === 'user.list:resp') {
        const payload = env.payload as any;
        const fetchedUsers = payload?.users || [];
        setUsers(fetchedUsers);
        setHasUsers(payload?.has_users || false);
        if (!payload?.has_users) {
          setIsSignup(true); // First-run: show signup
        }
      }
      if (env.topic === 'user.login:resp') {
        setLoading(false);
        const payload = env.payload as any;
        if (payload?.error) {
          setError(payload.error);
        } else if (payload?.success) {
          onLogin(payload.user);
        }
      }
      if (env.topic === 'user.signup:resp') {
        setLoading(false);
        const payload = env.payload as any;
        if (payload?.error) {
          setError(payload.error);
        } else if (payload?.success) {
          onLogin(payload.user);
        }
      }
    });
    return unsub;
  }, [onLogin]);

  const handleLogin = () => {
    if (!selectedUser || !password) return;
    setLoading(true);
    setError('');
    kernel.publish('user.login', { username: selectedUser.username, password });
  };

  const handleSignup = () => {
    if (!newUsername || !newPassword) return;
    setLoading(true);
    setError('');
    kernel.publish('user.signup', {
      username: newUsername,
      password: newPassword,
      display_name: newDisplayName || newUsername,
    });
  };

  return (
    <div className="h-screen w-screen bg-gradient-to-br from-[#0a0a1a] via-[#0d1025] to-[#0a0a1a] flex items-center justify-center select-none overflow-hidden">
      {/* Animated background particles */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        {Array.from({ length: 20 }).map((_, i) => (
          <div
            key={i}
            className="absolute rounded-full opacity-10"
            style={{
              width: `${Math.random() * 4 + 1}px`,
              height: `${Math.random() * 4 + 1}px`,
              background: `hsl(${200 + Math.random() * 60}, 80%, 70%)`,
              left: `${Math.random() * 100}%`,
              top: `${Math.random() * 100}%`,
              animation: `pulse ${3 + Math.random() * 4}s ease-in-out infinite`,
              animationDelay: `${Math.random() * 3}s`,
            }}
          />
        ))}
      </div>

      <div className="relative z-10 flex flex-col items-center gap-6 max-w-md w-full px-8">
        {/* Logo */}
        <div className="flex flex-col items-center gap-2 mb-4">
          <div className="w-16 h-16 rounded-2xl bg-gradient-to-br from-cyan-500 to-blue-600 flex items-center justify-center shadow-lg shadow-cyan-500/20">
            <Shield size={32} className="text-white" />
          </div>
          <h1 className="text-2xl font-bold text-white tracking-tight">Kernos OS</h1>
          <p className="text-xs text-gray-500">AI-Native Cognitive Operating System</p>
        </div>

        {/* ── Signup Mode (First Run or Manual) ── */}
        {isSignup ? (
          <div className="w-full bg-white/5 backdrop-blur-xl border border-white/10 rounded-2xl p-6 shadow-2xl">
            <h2 className="text-sm font-bold text-white mb-1">
              {hasUsers === false ? 'Create Your Account' : 'New Account'}
            </h2>
            <p className="text-xs text-gray-500 mb-5">
              {hasUsers === false
                ? 'Welcome to Kernos OS. Create your first administrator account.'
                : 'Create a new user account'}
            </p>

            <div className="space-y-3">
              <div className="relative">
                <User size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                <input
                  type="text"
                  placeholder="Username"
                  value={newUsername}
                  onChange={e => setNewUsername(e.target.value)}
                  className="w-full bg-black/40 border border-white/10 rounded-lg pl-9 pr-3 py-2.5 text-sm text-white placeholder-gray-600 outline-none focus:border-cyan-500/50 transition-colors"
                  autoFocus
                />
              </div>
              <div className="relative">
                <User size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                <input
                  type="text"
                  placeholder="Display Name (optional)"
                  value={newDisplayName}
                  onChange={e => setNewDisplayName(e.target.value)}
                  className="w-full bg-black/40 border border-white/10 rounded-lg pl-9 pr-3 py-2.5 text-sm text-white placeholder-gray-600 outline-none focus:border-cyan-500/50 transition-colors"
                />
              </div>
              <div className="relative">
                <Lock size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                <input
                  type={showPassword ? 'text' : 'password'}
                  placeholder="Password"
                  value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && handleSignup()}
                  className="w-full bg-black/40 border border-white/10 rounded-lg pl-9 pr-10 py-2.5 text-sm text-white placeholder-gray-600 outline-none focus:border-cyan-500/50 transition-colors"
                />
                <button
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-600 hover:text-white"
                >
                  {showPassword ? <EyeOff size={14} /> : <Eye size={14} />}
                </button>
              </div>
            </div>

            {error && (
              <div className="mt-3 text-xs text-red-400 bg-red-400/10 rounded-lg px-3 py-2">
                {error}
              </div>
            )}

            <button
              onClick={handleSignup}
              disabled={loading || !newUsername || !newPassword}
              className="w-full mt-4 py-2.5 rounded-lg bg-gradient-to-r from-cyan-500 to-blue-600 text-white text-sm font-bold hover:from-cyan-400 hover:to-blue-500 disabled:opacity-40 disabled:cursor-not-allowed transition-all flex items-center justify-center gap-2"
            >
              {loading ? <Loader2 size={14} className="animate-spin" /> : <Plus size={14} />}
              Create Account
            </button>

            <div className="flex items-center justify-between mt-4">
              {hasUsers && (
                <button
                  onClick={() => { setIsSignup(false); setError(''); }}
                  className="text-xs text-gray-500 hover:text-white transition-colors"
                >
                  ← Back to login
                </button>
              )}
              <button
                onClick={onGuestAccess}
                className="text-xs text-gray-600 hover:text-cyan-400 transition-colors ml-auto"
              >
                Continue as Guest
              </button>
            </div>
          </div>
        ) : (
          /* ── Login Mode ── */
          <div className="w-full">
            {/* User avatars */}
            <div className="flex flex-wrap items-center justify-center gap-4 mb-6">
              {users.map(user => (
                <button
                  key={user.id}
                  onClick={() => { setSelectedUser(user); setPassword(''); setError(''); }}
                  className={`flex flex-col items-center gap-2 p-3 rounded-xl transition-all ${
                    selectedUser?.id === user.id
                      ? 'bg-white/10 border border-cyan-500/30 scale-105'
                      : 'hover:bg-white/5 border border-transparent'
                  }`}
                >
                  <div className="w-16 h-16 rounded-full bg-gradient-to-br from-cyan-500/30 to-blue-600/30 border-2 border-white/10 flex items-center justify-center overflow-hidden">
                    {user.avatar_url ? (
                      <img src={user.avatar_url} alt={user.username} className="w-full h-full object-cover" />
                    ) : (
                      <User size={28} className="text-gray-400" />
                    )}
                  </div>
                  <span className="text-xs text-gray-300 font-medium">{user.display_name || user.username}</span>
                  <span className="text-[10px] text-gray-600">{user.role}</span>
                </button>
              ))}
            </div>

            {/* Password field (shown when user selected) */}
            {selectedUser && (
              <div className="bg-white/5 backdrop-blur-xl border border-white/10 rounded-2xl p-6 shadow-2xl">
                <div className="text-center mb-4">
                  <p className="text-sm text-white font-bold">{selectedUser.display_name || selectedUser.username}</p>
                  <p className="text-[10px] text-gray-500">Enter your password to continue</p>
                </div>

                <div className="relative">
                  <Lock size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                  <input
                    type={showPassword ? 'text' : 'password'}
                    placeholder="Password"
                    value={password}
                    onChange={e => setPassword(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && handleLogin()}
                    className="w-full bg-black/40 border border-white/10 rounded-lg pl-9 pr-10 py-2.5 text-sm text-white placeholder-gray-600 outline-none focus:border-cyan-500/50 transition-colors"
                    autoFocus
                  />
                  <button
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-600 hover:text-white"
                  >
                    {showPassword ? <EyeOff size={14} /> : <Eye size={14} />}
                  </button>
                </div>

                {error && (
                  <div className="mt-3 text-xs text-red-400 bg-red-400/10 rounded-lg px-3 py-2">
                    {error}
                  </div>
                )}

                <button
                  onClick={handleLogin}
                  disabled={loading || !password}
                  className="w-full mt-4 py-2.5 rounded-lg bg-gradient-to-r from-cyan-500 to-blue-600 text-white text-sm font-bold hover:from-cyan-400 hover:to-blue-500 disabled:opacity-40 disabled:cursor-not-allowed transition-all flex items-center justify-center gap-2"
                >
                  {loading ? <Loader2 size={14} className="animate-spin" /> : <LogIn size={14} />}
                  Sign In
                </button>
              </div>
            )}

            {/* Bottom actions */}
            <div className="flex items-center justify-between mt-5 px-2">
              <button
                onClick={() => { setIsSignup(true); setError(''); }}
                className="text-xs text-gray-500 hover:text-white transition-colors flex items-center gap-1"
              >
                <Plus size={12} /> New Account
              </button>
              <button
                onClick={onGuestAccess}
                className="text-xs text-gray-600 hover:text-cyan-400 transition-colors"
              >
                Continue as Guest
              </button>
            </div>
          </div>
        )}

        {/* Version */}
        <p className="text-[10px] text-gray-700 mt-4">Kernos OS v1.0 • Codestral 22B • Local-First AI</p>
      </div>

      <style>{`
        @keyframes pulse {
          0%, 100% { opacity: 0.05; transform: scale(1); }
          50% { opacity: 0.15; transform: scale(1.5); }
        }
      `}</style>
    </div>
  );
};
