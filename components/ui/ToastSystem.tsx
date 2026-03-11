import React, { useEffect, useState } from 'react';
import { kernel } from '../../services/kernel';
import { Envelope } from '../../types';
import { AlertOctagon, Bell, CheckCircle, AlertTriangle, Info, X } from 'lucide-react';

interface ToastMessage {
    id: string;
    title?: string;
    message: string;
    urgency: 'info' | 'success' | 'warning' | 'error' | 'panic';
}

const TOAST_COLORS = {
    info: { bg: 'bg-blue-950/90', border: 'border-blue-500', icon: Info, textHead: 'text-blue-200', textBody: 'text-blue-100' },
    success: { bg: 'bg-emerald-950/90', border: 'border-emerald-500', icon: CheckCircle, textHead: 'text-emerald-200', textBody: 'text-emerald-100' },
    warning: { bg: 'bg-amber-950/90', border: 'border-amber-500', icon: AlertTriangle, textHead: 'text-amber-200', textBody: 'text-amber-100' },
    error: { bg: 'bg-red-950/90', border: 'border-red-500', icon: AlertOctagon, textHead: 'text-red-200', textBody: 'text-red-100' },
    panic: { bg: 'bg-red-950/90', border: 'border-red-500', icon: AlertOctagon, textHead: 'text-red-200', textBody: 'text-red-100' },
};

export const ToastSystem: React.FC = () => {
    const [toasts, setToasts] = useState<ToastMessage[]>([]);

    useEffect(() => {
        const unsubscribe = kernel.subscribe((env: Envelope) => {
            // Legacy: Kernel panic toasts
            if (env.topic === 'sys.kernel_panic') {
                const payload = env.payload as Record<string, string>;
                addToast({
                    title: 'Kernel Panic Intercepted',
                    message: `${payload.subsystem}: ${payload.error}`,
                    urgency: 'panic',
                });
            }

            // Phase 8: General notification toasts
            if (env.topic === 'sys.notify:toast') {
                const payload = env.payload as Record<string, string>;
                addToast({
                    title: payload.title,
                    message: payload.message || 'Notification',
                    urgency: (payload.urgency as ToastMessage['urgency']) || 'info',
                });
            }
        });
        return unsubscribe;
    }, []);

    const addToast = (toast: Omit<ToastMessage, 'id'>) => {
        const id = Math.random().toString(36).substring(7);
        const newToast = { ...toast, id };
        setToasts(prev => [...prev, newToast]);
        setTimeout(() => {
            setToasts(prev => prev.filter(t => t.id !== id));
        }, 6000);
    };

    if (toasts.length === 0) return null;

    return (
        <div className="fixed top-4 right-4 z-[9999] flex flex-col gap-2 max-w-sm pointer-events-none">
            {toasts.map(toast => {
                const style = TOAST_COLORS[toast.urgency] || TOAST_COLORS.info;
                const IconComponent = style.icon;
                return (
                    <div
                        key={toast.id}
                        className={`pointer-events-auto ${style.bg} border-l-4 ${style.border} p-4 rounded-lg shadow-2xl backdrop-blur-sm flex items-start gap-3`}
                        style={{ animation: 'slideIn 0.3s ease-out' }}
                    >
                        <IconComponent className={`${style.textHead} shrink-0 mt-0.5`} size={20} />
                        <div className="flex-1 min-w-0">
                            {toast.title && (
                                <h4 className={`font-bold ${style.textHead} text-sm mb-1 uppercase tracking-wider`}>
                                    {toast.title}
                                </h4>
                            )}
                            <p className={`text-xs ${style.textBody} font-mono line-clamp-3`}>{toast.message}</p>
                        </div>
                        <button
                            onClick={() => setToasts(prev => prev.filter(t => t.id !== toast.id))}
                            className={`${style.textHead} opacity-50 hover:opacity-100 shrink-0`}
                        >
                            <X size={16} />
                        </button>
                    </div>
                );
            })}
            <style>{`
                @keyframes slideIn {
                    from { opacity: 0; transform: translateX(100px); }
                    to { opacity: 1; transform: translateX(0); }
                }
            `}</style>
        </div>
    );
};
