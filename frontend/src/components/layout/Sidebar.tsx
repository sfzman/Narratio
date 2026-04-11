import React from 'react';
import { 
  Folder, 
  ChevronDown, 
  ChevronRight, 
  Plus, 
  Settings, 
  LayoutGrid, 
  FileText, 
  Image as ImageIcon, 
  Music, 
  Video,
  Layers,
  Search
} from 'lucide-react';
import { cn } from '../../lib/utils';

interface SidebarProps {
  activeTaskId: string | null;
  onTaskSelect: (taskId: string) => void;
  onCreateTask: () => void;
}

const Sidebar = ({ activeTaskId, onTaskSelect, onCreateTask }: SidebarProps) => {
  const [expandedProjects, setExpandedProjects] = React.useState<string[]>(['p1']);

  const toggleProject = (id: string) => {
    setExpandedProjects(prev => 
      prev.includes(id) ? prev.filter(p => p !== id) : [...prev, id]
    );
  };

  const projects = [
    {
      id: 'p1',
      name: 'Cyberpunk_Odyssey',
      tasks: [
        { id: 'segmentation', name: 'Segmentation', icon: FileText },
        { id: 'outline', name: 'Outline', icon: LayoutGrid },
        { id: 'character_sheet', name: 'Character Sheet', icon: Layers },
        { id: 'script', name: 'Script', icon: FileText },
        { id: 'character_image', name: 'Character Image', icon: ImageIcon },
        { id: 'tts', name: 'TTS Clips', icon: Music },
        { id: 'image', name: 'Image Assets', icon: ImageIcon },
        { id: 'shot_video', name: 'Shot Video', icon: Video },
        { id: 'video', name: 'Final Video', icon: Video },
      ]
    },
    { id: 'p2', name: 'Neo_Tokyo_Drift', tasks: [] },
    { id: 'p3', name: 'The_Last_Synthesizer', tasks: [] },
  ];

  return (
    <div className="w-64 h-full bg-slate-950 border-r border-slate-900 flex flex-col">
      <div className="p-6 flex items-center gap-3 border-b border-slate-900">
        <div className="w-8 h-8 bg-brand-cyan rounded flex items-center justify-center">
          <span className="text-slate-950 font-black text-xl">N</span>
        </div>
        <h1 className="font-bold text-lg tracking-tight text-slate-100">NARRATIO</h1>
      </div>

      <div className="p-4">
        <div className="relative">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-600" />
          <input 
            type="text" 
            placeholder="Search projects..." 
            className="w-full bg-slate-900/50 border border-slate-800 rounded-lg py-2 pl-10 pr-4 text-xs text-slate-300 focus:outline-none focus:border-brand-cyan/50 transition-colors"
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto py-2">
        <div className="px-4 mb-4 flex items-center justify-between">
          <span className="text-[10px] font-bold text-slate-600 uppercase tracking-widest">Projects</span>
          <button 
            onClick={onCreateTask}
            className="p-1 hover:bg-slate-900 rounded text-slate-500 transition-colors"
          >
            <Plus className="w-4 h-4" />
          </button>
        </div>

        <div className="space-y-1">
          {projects.map(project => (
            <div key={project.id} className="px-2">
              <button 
                onClick={() => toggleProject(project.id)}
                className={cn(
                  "w-full flex items-center gap-2 px-3 py-2 rounded-lg transition-colors group",
                  expandedProjects.includes(project.id) ? "bg-slate-900/50 text-slate-100" : "text-slate-500 hover:text-slate-300"
                )}
              >
                {expandedProjects.includes(project.id) ? (
                  <ChevronDown className="w-4 h-4" />
                ) : (
                  <ChevronRight className="w-4 h-4" />
                )}
                <Folder className={cn("w-4 h-4", expandedProjects.includes(project.id) ? "text-brand-cyan" : "text-slate-600")} />
                <span className="text-sm font-medium truncate">{project.name}</span>
              </button>

              {expandedProjects.includes(project.id) && (
                <div className="mt-1 ml-4 space-y-0.5 border-l border-slate-900">
                  {project.tasks.map(task => {
                    const isActive = activeTaskId === task.id;
                    return (
                      <button
                        key={task.id}
                        onClick={() => onTaskSelect(task.id)}
                        className={cn(
                          "w-full flex items-center gap-3 px-4 py-2 text-xs transition-all relative",
                          isActive 
                            ? "text-slate-100 font-semibold bg-slate-900/30" 
                            : "text-slate-500 hover:text-slate-300"
                        )}
                      >
                        {isActive && (
                          <div className="absolute left-0 top-0 bottom-0 w-0.5 bg-brand-cyan" />
                        )}
                        <task.icon className={cn("w-3.5 h-3.5", isActive ? "text-brand-cyan" : "text-slate-700")} />
                        <span className="truncate">{task.name}</span>
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      <div className="p-4 border-t border-slate-900">
        <button className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-slate-500 hover:text-slate-300 hover:bg-slate-900 transition-all group">
          <Settings className="w-4 h-4 group-hover:rotate-90 transition-transform duration-500" />
          <span className="text-sm font-medium">Workspace Settings</span>
        </button>
      </div>
    </div>
  );
};

export default Sidebar;
