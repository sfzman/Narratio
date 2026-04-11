/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useState, useCallback } from 'react';
import Sidebar from './components/layout/Sidebar';
import WorkflowCanvas from './components/workflow/WorkflowCanvas';
import NodeInspector from './components/workflow/NodeInspector';
import CreateTaskModal from './components/modals/CreateTaskModal';
import { WorkflowNode } from './types/workflow';
import { DEFAULT_NODES } from './constants/workflow';
import { Bell, Settings, User, Plus, Play, ChevronRight } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';

export default function App() {
  const [selectedNode, setSelectedNode] = useState<WorkflowNode | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);

  const handleTaskSelect = useCallback((taskId: string) => {
    const node = DEFAULT_NODES.find(n => n.id === taskId);
    if (node) {
      setSelectedNode(node);
    }
  }, []);

  return (
    <div className="flex h-screen w-full bg-slate-950 overflow-hidden font-sans">
      {/* Sidebar */}
      <Sidebar 
        activeTaskId={selectedNode?.id || null} 
        onTaskSelect={handleTaskSelect} 
        onCreateTask={() => setIsCreateModalOpen(true)}
      />

      {/* Main Content Area */}
      <div className="flex-1 flex flex-col relative min-w-0">
        {/* Header */}
        <header className="h-16 border-b border-slate-900 flex items-center justify-between px-8 bg-slate-950/50 backdrop-blur-md z-20">
          <div className="flex items-center gap-4 text-xs font-medium">
            <span className="text-slate-500 uppercase tracking-widest">NARRATIO</span>
            <span className="text-slate-700">/</span>
            <span className="text-slate-500 uppercase tracking-widest">PROJECTS</span>
            <span className="text-slate-700">/</span>
            <span className="text-slate-100 font-bold tracking-widest">CYBERPUNK_ODYSSEY</span>
            <div className="flex items-center gap-2 ml-4 px-2 py-1 bg-brand-cyan/10 border border-brand-cyan/20 rounded-md">
              <div className="w-1.5 h-1.5 rounded-full bg-brand-cyan animate-pulse" />
              <span className="text-[10px] font-bold text-brand-cyan uppercase tracking-widest">RUNNING</span>
            </div>
          </div>

          <div className="flex items-center gap-6">
          <div className="flex items-center gap-4">
            <button className="text-slate-500 hover:text-slate-300 transition-colors">
              <Bell className="w-5 h-5" />
            </button>
            <button className="text-slate-500 hover:text-slate-300 transition-colors">
              <Settings className="w-5 h-5" />
            </button>
            <div className="w-8 h-8 rounded-full bg-slate-800 border border-slate-700 flex items-center justify-center overflow-hidden ml-2">
                <img 
                  src="https://picsum.photos/seed/user/100/100" 
                  alt="User" 
                  className="w-full h-full object-cover"
                  referrerPolicy="no-referrer"
                />
              </div>
            </div>
          </div>
        </header>

        {/* Canvas Area */}
        <main className="flex-1 relative overflow-hidden">
          <WorkflowCanvas 
            selectedNodeId={selectedNode?.id || null} 
            onNodeSelect={setSelectedNode} 
          />
        </main>

        {/* Node Inspector Overlay */}
        <AnimatePresence>
          {selectedNode && (
            <div className="absolute inset-y-0 right-0 z-40">
              <NodeInspector node={selectedNode} onClose={() => setSelectedNode(null)} />
            </div>
          )}
        </AnimatePresence>
      </div>

      {/* Modals */}
      <CreateTaskModal 
        isOpen={isCreateModalOpen} 
        onClose={() => setIsCreateModalOpen(false)} 
        onCreate={(params) => {
          console.log('Creating task with params:', params);
          setIsCreateModalOpen(false);
        }}
      />
    </div>
  );
}
